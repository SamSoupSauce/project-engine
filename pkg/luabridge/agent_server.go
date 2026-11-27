package luabridge

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SamSoupSauce/project-engine/engine"
	"github.com/gorilla/websocket"
)

const (
	// DefaultAgentServerAddress is the default bind address for the dual-transport server.
	DefaultAgentServerAddress = "127.0.0.1:4242"

	defaultCommandQueueSize = 256
	defaultWSPingPeriod     = 25 * time.Second
	defaultWSPongWait       = 60 * time.Second
	defaultWSWriteWait      = 10 * time.Second
)

// FrameCaptureResult holds a base64-encoded PNG/JPEG frame produced by engine.capture_frame.
type FrameCaptureResult struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Data   string `json:"data"`
}

// AgentServerConfig configures AgentServer transport, authentication, and keepalives.
type AgentServerConfig struct {
	Address    string
	Token      string
	PingPeriod time.Duration
	PongWait   time.Duration
	WriteWait  time.Duration
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcSuccessResult struct {
	Status   string   `json:"status"`
	Data     any      `json:"data"`
	Logs     []string `json:"logs"`
	Tick     uint64   `json:"tick"`
	TimingMs float64  `json:"timing_ms"`
}

type rpcResponse struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      any               `json:"id"`
	Result  *rpcSuccessResult `json:"result,omitempty"`
	Error   *rpcError         `json:"error,omitempty"`
}

type wsInboundFrame struct {
	ID     any             `json:"id"`
	Op     string          `json:"op"`
	Cmd    string          `json:"cmd,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type wsOutboundFrame struct {
	ID    any    `json:"id"`
	OK    bool   `json:"ok"`
	Res   any    `json:"res"`
	Error string `json:"error,omitempty"`
}

type wsLogEvent struct {
	Event string `json:"event"`
	Msg   string `json:"msg"`
}

type agentResponse struct {
	Data     any
	Logs     []string
	Tick     uint64
	TimingMs float64
	Err      *rpcError
}

type agentCommand struct {
	sender string
	method string
	params json.RawMessage
	reply  chan agentResponse
}

type pendingFrameCapture struct {
	format  string
	start   time.Time
	tick    uint64
	logs    []string
	replyCh chan agentResponse
}

type wsClient struct {
	conn   *websocket.Conn
	send   chan []byte
	server *AgentServer
	sender string
}

// AgentServer provides concurrent HTTP JSON-RPC (/rpc) and WebSocket (/ws) access
// to a running engine instance by dispatching all commands through an MPSC CommandQueue
// drained synchronously on the main engine Tick() loop.
type AgentServer struct {
	Bridge            *LuaBridge
	Console           *Console
	Scene             *engine.BaseScene
	Address           string
	Token             string
	CommandQueue      chan agentCommand
	StartedAt         time.Time
	DeferFrameCapture bool
	FrameCapturer     func(format string) (FrameCaptureResult, error)

	pingPeriod time.Duration
	pongWait   time.Duration
	writeWait  time.Duration

	upgrader   websocket.Upgrader
	httpServer *http.Server
	listener   net.Listener

	clientsMu sync.RWMutex
	clients   map[*wsClient]struct{}

	captureMu      sync.Mutex
	activeLogSink  *[]string
	pendingCapture []pendingFrameCapture

	lifecycleMu sync.Mutex
	running     bool
	stopCh      chan struct{}
}

// NewAgentServer initializes an AgentServer and registers its CommandQueue drain hook
// at the top of scene.Tick() when scene is non-nil.
func NewAgentServer(bridge *LuaBridge, console *Console, scene *engine.BaseScene, cfg AgentServerConfig) *AgentServer {
	addr := strings.TrimSpace(cfg.Address)
	if addr == "" {
		addr = DefaultAgentServerAddress
	}
	pingPeriod := cfg.PingPeriod
	if pingPeriod <= 0 {
		pingPeriod = defaultWSPingPeriod
	}
	pongWait := cfg.PongWait
	if pongWait <= 0 {
		pongWait = defaultWSPongWait
	}
	writeWait := cfg.WriteWait
	if writeWait <= 0 {
		writeWait = defaultWSWriteWait
	}
	if bridge == nil && console != nil {
		bridge = console.Bridge
	}
	if scene == nil && bridge != nil {
		bridge.dataMu.RLock()
		scene = bridge.scene
		bridge.dataMu.RUnlock()
	}

	s := &AgentServer{
		Bridge:       bridge,
		Console:      console,
		Scene:        scene,
		Address:      addr,
		Token:        cfg.Token,
		CommandQueue: make(chan agentCommand, defaultCommandQueueSize),
		StartedAt:    time.Now(),
		pingPeriod:   pingPeriod,
		pongWait:     pongWait,
		writeWait:    writeWait,
		clients:      make(map[*wsClient]struct{}),
		stopCh:       make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Allow Cloudflare Tunnel / external proxy origins; requests are gated by token auth.
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}

	if console != nil {
		console.AddLogSink(s.onConsoleLog)
	} else if bridge != nil {
		bridge.AddLogSink(s.onConsoleLog)
	}

	if scene != nil {
		scene.AddPreTickHook(func(_ int) {
			s.Drain()
		})
	}

	return s
}

// Start binds the TCP listener and begins serving HTTP JSON-RPC (/rpc) and WebSocket (/ws)
// concurrently without blocking the caller.
func (s *AgentServer) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.running {
		return fmt.Errorf("agentserver: already running on %s", s.Address)
	}

	ln, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("agentserver: listen %q: %w", s.Address, err)
	}
	s.listener = ln
	s.Address = ln.Addr().String()
	s.stopCh = make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleHTTPJRPC)
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.running = true

	go func() {
		err := s.httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			if s.Console != nil {
				s.Console.Print(fmt.Sprintf("[AgentServer] Serve error: %v", err), LineError)
			}
		}
	}()
	return nil
}

// BoundAddress returns the active listener address (useful when started with port :0).
func (s *AgentServer) BoundAddress() string {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.Address
}

// Stop shuts down the HTTP/WebSocket server and closes active client connections.
func (s *AgentServer) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	if !s.running {
		s.lifecycleMu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopCh)
	srv := s.httpServer
	s.lifecycleMu.Unlock()

	s.clientsMu.Lock()
	for client := range s.clients {
		delete(s.clients, client)
		close(client.send)
		_ = client.conn.Close()
	}
	s.clientsMu.Unlock()

	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// Drain executes all currently queued commands synchronously on the calling (main tick) thread.
func (s *AgentServer) Drain() {
	if s == nil {
		return
	}
	for {
		select {
		case cmd := <-s.CommandQueue:
			s.executeOnMainThread(cmd)
		default:
			return
		}
	}
}

// FlushFrameCaptures fulfills any queued post-render capture_frame requests on the main OpenGL thread
// right before SwapBuffers.
func (s *AgentServer) FlushFrameCaptures(width, height int) {
	if s == nil || len(s.pendingCapture) == 0 {
		return
	}
	pending := s.pendingCapture
	s.pendingCapture = nil

	for _, req := range pending {
		var res FrameCaptureResult
		var err error
		if s.FrameCapturer != nil {
			res, err = s.FrameCapturer(req.format)
		} else {
			var b64, normFmt string
			b64, normFmt, err = engine.CaptureGLBackbuffer(width, height, req.format)
			if err == nil {
				res = FrameCaptureResult{
					Format: normFmt,
					Width:  width,
					Height: height,
					Data:   b64,
				}
			}
		}
		timingMs := float64(time.Since(req.start).Microseconds()) / 1000.0
		if err != nil {
			req.replyCh <- agentResponse{
				Logs:     req.logs,
				Tick:     req.tick,
				TimingMs: timingMs,
				Err: &rpcError{
					Code:    -32001,
					Message: err.Error(),
					Data:    map[string]any{"logs": req.logs},
				},
			}
			continue
		}
		req.replyCh <- agentResponse{
			Data:     res,
			Logs:     req.logs,
			Tick:     req.tick,
			TimingMs: timingMs,
		}
	}
}

func (s *AgentServer) enqueueAndWait(ctx context.Context, sender, method string, params json.RawMessage) (agentResponse, error) {
	cmd := agentCommand{
		sender: sender,
		method: method,
		params: params,
		reply:  make(chan agentResponse, 1),
	}

	s.lifecycleMu.Lock()
	stopCh := s.stopCh
	s.lifecycleMu.Unlock()

	select {
	case s.CommandQueue <- cmd:
	case <-ctx.Done():
		return agentResponse{}, ctx.Err()
	case <-stopCh:
		return agentResponse{}, fmt.Errorf("agent server stopped")
	}

	select {
	case resp := <-cmd.reply:
		return resp, nil
	case <-ctx.Done():
		return agentResponse{}, ctx.Err()
	case <-stopCh:
		return agentResponse{}, fmt.Errorf("agent server stopped")
	}
}

func (s *AgentServer) executeOnMainThread(cmd agentCommand) {
	start := time.Now()
	var captured []string

	s.captureMu.Lock()
	prevSink := s.activeLogSink
	s.activeLogSink = &captured
	s.captureMu.Unlock()

	defer func() {
		s.captureMu.Lock()
		s.activeLogSink = prevSink
		s.captureMu.Unlock()
	}()

	var tick uint64
	if s.Scene != nil {
		tick = s.Scene.GetTickCount()
	}

	if cmd.method == "engine.capture_frame" {
		var p struct {
			Format string `json:"format"`
		}
		if len(cmd.params) > 0 && string(cmd.params) != "null" {
			if err := json.Unmarshal(cmd.params, &p); err != nil {
				cmd.reply <- agentResponse{
					Tick:     tick,
					TimingMs: float64(time.Since(start).Microseconds()) / 1000.0,
					Err:      &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)},
				}
				return
			}
		}
		if s.DeferFrameCapture {
			logsCopy := append([]string(nil), captured...)
			s.pendingCapture = append(s.pendingCapture, pendingFrameCapture{
				format:  p.Format,
				start:   start,
				tick:    tick,
				logs:    logsCopy,
				replyCh: cmd.reply,
			})
			return
		}
		if s.FrameCapturer == nil {
			cmd.reply <- agentResponse{
				Tick:     tick,
				TimingMs: float64(time.Since(start).Microseconds()) / 1000.0,
				Err:      &rpcError{Code: -32001, Message: "no active OpenGL window available for frame capture"},
			}
			return
		}
		res, err := s.FrameCapturer(p.Format)
		if err != nil {
			cmd.reply <- agentResponse{
				Tick:     tick,
				TimingMs: float64(time.Since(start).Microseconds()) / 1000.0,
				Err:      &rpcError{Code: -32001, Message: err.Error()},
			}
			return
		}
		cmd.reply <- agentResponse{
			Data:     res,
			Logs:     append([]string{}, captured...),
			Tick:     tick,
			TimingMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
		return
	}

	data, rpcErr := s.dispatchMethod(cmd.sender, cmd.method, cmd.params)
	logsCopy := make([]string, len(captured))
	copy(logsCopy, captured)

	if rpcErr != nil && rpcErr.Data == nil && len(logsCopy) > 0 {
		rpcErr.Data = map[string]any{"logs": logsCopy}
	}

	cmd.reply <- agentResponse{
		Data:     data,
		Logs:     logsCopy,
		Tick:     tick,
		TimingMs: float64(time.Since(start).Microseconds()) / 1000.0,
		Err:      rpcErr,
	}
}

func (s *AgentServer) dispatchMethod(sender, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "engine.ping":
		sceneName := ""
		objCount := 0
		if s.Scene != nil {
			sceneName = s.Scene.GetName()
			objCount = s.Scene.ObjectCount()
		}
		var tick uint64
		if s.Scene != nil {
			tick = s.Scene.GetTickCount()
		}
		uptime := time.Since(s.StartedAt)
		return map[string]any{
			"status":       "ok",
			"uptime_ms":    uptime.Milliseconds(),
			"uptime_sec":   uptime.Seconds(),
			"tick":         tick,
			"scene":        sceneName,
			"object_count": objCount,
		}, nil

	case "engine.console_exec":
		cmdText, err := extractConsoleCommandParam(params)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		if s.Bridge == nil {
			return nil, &rpcError{Code: -32000, Message: "LuaBridge not initialized"}
		}
		if strings.TrimSpace(sender) == "" {
			sender = "agent:rpc"
		}
		execCtx := ExecutionContext{
			Sender:    sender,
			Timestamp: time.Now(),
			Raw:       cmdText,
		}
		out, execErr := s.Bridge.ExecuteWithContext(execCtx, cmdText)
		if execErr != nil {
			return nil, &rpcError{
				Code:    -32000,
				Message: execErr.Error(),
				Data: map[string]any{
					"traceback": execErr.Error(),
				},
			}
		}
		return map[string]any{
			"output": out,
		}, nil

	case "engine.query_prefabs":
		if s.Bridge == nil || s.Bridge.PrefabEditor() == nil {
			return nil, &rpcError{Code: -32000, Message: "PrefabService not initialized"}
		}
		var p struct {
			Query string `json:"query"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
			}
		}
		list, err := s.Bridge.PrefabEditor().SearchPrefabs(p.Query)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		if list == nil {
			list = []engine.PrefabSummary{}
		}
		return map[string]any{
			"prefabs": list,
			"count":   len(list),
		}, nil

	case "engine.query_components":
		if s.Bridge == nil {
			return nil, &rpcError{Code: -32000, Message: "LuaBridge not initialized"}
		}
		var p struct {
			Query string `json:"query"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
			}
		}
		list := s.Bridge.SearchComponents(p.Query)
		if list == nil {
			list = []ComponentSearchResult{}
		}
		return map[string]any{
			"components": list,
			"count":      len(list),
		}, nil

	case "engine.query_docs":
		if s.Bridge == nil {
			return nil, &rpcError{Code: -32000, Message: "LuaBridge not initialized"}
		}
		var topic string
		trimmedParams := strings.TrimSpace(string(params))
		if trimmedParams != "" && trimmedParams != "null" {
			if strings.HasPrefix(trimmedParams, "\"") {
				if err := json.Unmarshal(params, &topic); err != nil {
					return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
				}
			} else {
				var p struct {
					Topic string `json:"topic"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
				}
				topic = p.Topic
			}
		}
		res, err := s.Bridge.QueryDocs(topic)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		return res, nil

	case "engine.spawn_prefab":
		if s.Bridge == nil || s.Bridge.PrefabEditor() == nil {
			return nil, &rpcError{Code: -32000, Message: "PrefabService not initialized"}
		}
		var p struct {
			Name       string   `json:"name"`
			ID         string   `json:"id"`
			ObjectName string   `json:"object_name"`
			X          *float32 `json:"x"`
			Y          *float32 `json:"y"`
			Z          *float32 `json:"z"`
			Pick       bool     `json:"pick"`
			Position   *struct {
				X float32 `json:"x"`
				Y float32 `json:"y"`
				Z float32 `json:"z"`
			} `json:"position"`
		}
		if len(params) == 0 || string(params) == "null" {
			return nil, &rpcError{Code: -32602, Message: "spawn_prefab requires params.name"}
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
		}
		if strings.TrimSpace(p.Name) == "" {
			return nil, &rpcError{Code: -32602, Message: "spawn_prefab requires non-empty name"}
		}
		opts := PrefabSpawnOptions{
			ID:          p.ID,
			Name:        p.ObjectName,
			AutoAdd:     true,
			PickSpawned: p.Pick,
		}
		if p.Position != nil {
			pos := engine.NewVector3(p.Position.X, p.Position.Y, p.Position.Z)
			opts.Position = &pos
		} else if p.X != nil || p.Y != nil || p.Z != nil {
			var x, y, z float32
			if p.X != nil {
				x = *p.X
			}
			if p.Y != nil {
				y = *p.Y
			}
			if p.Z != nil {
				z = *p.Z
			}
			pos := engine.NewVector3(x, y, z)
			opts.Position = &pos
		}

		obj, err := s.Bridge.PrefabEditor().SpawnPrefabObject(p.Name, opts)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		pos := obj.GetWorldTransform().Position
		compNames := make([]string, 0, len(obj.Components))
		for _, c := range obj.Components {
			if c != nil {
				compNames = append(compNames, componentCanonicalName(c))
			}
		}
		return map[string]any{
			"id":         obj.ID,
			"name":       obj.Name,
			"position":   vec3JSON{X: pos.X, Y: pos.Y, Z: pos.Z},
			"components": compNames,
		}, nil

	default:
		return nil, &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", method)}
	}
}

func extractConsoleCommandParam(params json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(params))
	if trimmed == "" || trimmed == "null" {
		return "", fmt.Errorf("console_exec requires cmd parameter")
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(params, &s); err != nil {
			return "", err
		}
		if strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("console_exec cmd must not be empty")
		}
		return s, nil
	}
	var obj struct {
		Cmd     string `json:"cmd"`
		Command string `json:"command"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(params, &obj); err != nil {
		return "", fmt.Errorf("invalid params: %v", err)
	}
	cmd := obj.Cmd
	if cmd == "" {
		cmd = obj.Command
	}
	if cmd == "" {
		cmd = obj.Code
	}
	if strings.TrimSpace(cmd) == "" {
		return "", fmt.Errorf("console_exec requires non-empty cmd")
	}
	return cmd, nil
}

func (s *AgentServer) authenticateRequest(r *http.Request, allowQuery bool) bool {
	tok, present := extractRequestToken(r, allowQuery)
	if !present {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.Token)) == 1
}

func extractRequestToken(r *http.Request, allowQuery bool) (string, bool) {
	if vals, ok := r.Header[http.CanonicalHeaderKey("Authorization")]; ok && len(vals) > 0 {
		auth := vals[0]
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer "), true
		}
		if strings.EqualFold(strings.TrimSpace(auth), "Bearer") {
			return "", true
		}
		return "", false
	}
	if vals, ok := r.Header[http.CanonicalHeaderKey("X-Engine-Token")]; ok && len(vals) > 0 {
		return vals[0], true
	}
	if allowQuery {
		q := r.URL.Query()
		if vals, ok := q["token"]; ok && len(vals) > 0 {
			return vals[0], true
		}
	}
	return "", false
}

func (s *AgentServer) handleHTTPJRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authenticateRequest(r, false) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeJSONRPCError(w, nil, -32700, "failed reading request body", nil)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, -32700, fmt.Sprintf("parse error: %v", err), nil)
		return
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		writeJSONRPCError(w, req.ID, -32600, "invalid JSON-RPC 2.0 request", nil)
		return
	}

	sender := resolveRPCClientSender(r, req.Params)
	resp, err := s.enqueueAndWait(r.Context(), sender, req.Method, req.Params)
	if err != nil {
		writeJSONRPCError(w, req.ID, -32000, err.Error(), nil)
		return
	}
	if resp.Err != nil {
		writeJSONRPCError(w, req.ID, resp.Err.Code, resp.Err.Message, resp.Err.Data)
		return
	}

	logs := resp.Logs
	if logs == nil {
		logs = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: &rpcSuccessResult{
			Status:   "success",
			Data:     resp.Data,
			Logs:     logs,
			Tick:     resp.Tick,
			TimingMs: resp.TimingMs,
		},
	})
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	})
}

func (s *AgentServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.authenticateRequest(r, true) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &wsClient{
		conn:   conn,
		send:   make(chan []byte, 256),
		server: s,
		sender: resolveWSClientSender(r),
	}

	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()

	go client.writePump()
	client.readPump(r.Context())
}

func (s *AgentServer) removeWSClient(c *wsClient) {
	s.clientsMu.Lock()
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		close(c.send)
	}
	s.clientsMu.Unlock()
}

func (c *wsClient) readPump(reqCtx context.Context) {
	defer func() {
		c.server.removeWSClient(c)
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(c.server.pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.server.pongWait))
	})

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var frame wsInboundFrame
		if err := json.Unmarshal(payload, &frame); err != nil {
			c.enqueueOutbound(wsOutboundFrame{
				ID:    nil,
				OK:    false,
				Res:   nil,
				Error: fmt.Sprintf("invalid JSON frame: %v", err),
			})
			continue
		}

		method, params, convErr := normalizeWSFrame(frame)
		if convErr != nil {
			c.enqueueOutbound(wsOutboundFrame{
				ID:    frame.ID,
				OK:    false,
				Res:   nil,
				Error: convErr.Error(),
			})
			continue
		}

		resp, err := c.server.enqueueAndWait(reqCtx, c.sender, method, params)
		if err != nil {
			c.enqueueOutbound(wsOutboundFrame{
				ID:    frame.ID,
				OK:    false,
				Res:   nil,
				Error: err.Error(),
			})
			continue
		}
		if resp.Err != nil {
			c.enqueueOutbound(wsOutboundFrame{
				ID:    frame.ID,
				OK:    false,
				Res:   nil,
				Error: resp.Err.Message,
			})
			continue
		}

		var resVal any = resp.Data
		if method == "engine.console_exec" {
			if m, ok := resp.Data.(map[string]any); ok {
				outStr, _ := m["output"].(string)
				if outStr == "" || outStr == "Nil" {
					resVal = nil
				} else {
					resVal = outStr
				}
			}
		}
		c.enqueueOutbound(wsOutboundFrame{
			ID:  frame.ID,
			OK:  true,
			Res: resVal,
		})
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(c.server.pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.server.writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.server.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *wsClient) enqueueOutbound(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.server.clientsMu.RLock()
	_, exists := c.server.clients[c]
	if exists {
		select {
		case c.send <- raw:
		default:
		}
	}
	sinksDone := !exists
	c.server.clientsMu.RUnlock()
	_ = sinksDone
}

func normalizeWSFrame(frame wsInboundFrame) (string, json.RawMessage, error) {
	op := strings.ToLower(strings.TrimSpace(frame.Op))
	switch op {
	case "exec", "shell", "console_exec":
		if strings.TrimSpace(frame.Cmd) != "" {
			b, _ := json.Marshal(map[string]string{"cmd": frame.Cmd})
			return "engine.console_exec", b, nil
		}
		return "engine.console_exec", frame.Params, nil
	case "rpc":
		if strings.TrimSpace(frame.Method) == "" {
			return "", nil, fmt.Errorf("op 'rpc' requires method")
		}
		return frame.Method, frame.Params, nil
	case "ping", "query_prefabs", "query_components", "query_docs", "spawn_prefab", "capture_frame":
		return "engine." + op, frame.Params, nil
	default:
		if strings.HasPrefix(op, "engine.") {
			return op, frame.Params, nil
		}
		return "", nil, fmt.Errorf("unsupported ws op %q", frame.Op)
	}
}

func resolveRPCClientSender(r *http.Request, params json.RawMessage) string {
	if r != nil {
		if cid := strings.TrimSpace(r.Header.Get("X-Client-ID")); cid != "" {
			return normalizeAgentSender(cid)
		}
	}
	if len(params) > 0 && string(params) != "null" {
		var p struct {
			ClientID string `json:"client_id"`
		}
		if err := json.Unmarshal(params, &p); err == nil && strings.TrimSpace(p.ClientID) != "" {
			return normalizeAgentSender(p.ClientID)
		}
	}
	return "agent:rpc"
}

func resolveWSClientSender(r *http.Request) string {
	if r != nil {
		if cid := strings.TrimSpace(r.URL.Query().Get("client_id")); cid != "" {
			return normalizeAgentSender(cid)
		}
		if cid := strings.TrimSpace(r.Header.Get("X-Client-ID")); cid != "" {
			return normalizeAgentSender(cid)
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil && host != "" {
			return "agent:ws:" + host
		}
		if r.RemoteAddr != "" {
			return "agent:ws:" + r.RemoteAddr
		}
	}
	return "agent:ws"
}

func normalizeAgentSender(clientID string) string {
	trimmed := strings.TrimSpace(clientID)
	if trimmed == "" {
		return "agent:rpc"
	}
	if strings.HasPrefix(trimmed, "agent:") {
		return trimmed
	}
	return "agent:" + trimmed
}

func (s *AgentServer) onConsoleLog(line string) {
	if s == nil {
		return
	}
	s.captureMu.Lock()
	if s.activeLogSink != nil {
		*s.activeLogSink = append(*s.activeLogSink, line)
	}
	s.captureMu.Unlock()

	raw, err := json.Marshal(wsLogEvent{
		Event: "log",
		Msg:   line,
	})
	if err != nil {
		return
	}

	s.clientsMu.RLock()
	for client := range s.clients {
		select {
		case client.send <- raw:
		default:
		}
	}
	s.clientsMu.RUnlock()
}
