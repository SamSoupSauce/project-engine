package luabridge

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var tryCloudflareURLPattern = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// TunnelConfig configures automated cloudflared discovery, download, and quick-tunnel startup.
type TunnelConfig struct {
	TargetAddress string
	CacheDir      string
	BinaryPath    string
	DownloadURL   string
	HTTPClient    *http.Client
	OnURL         func(url string)
}

// TunnelManager manages a local cloudflared quick tunnel pointing at the running AgentServer.
type TunnelManager struct {
	cfg     TunnelConfig
	console *Console

	mu     sync.RWMutex
	url    string
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
}

// NewTunnelManager creates a TunnelManager bound to console and config.
func NewTunnelManager(console *Console, cfg TunnelConfig) *TunnelManager {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &TunnelManager{
		cfg:     cfg,
		console: console,
	}
}

// GetURL returns the active trycloudflare.com URL once assigned, or "".
func (m *TunnelManager) GetURL() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.url
}

// ParseTryCloudflareURL extracts the first https://<id>.trycloudflare.com URL from a log line.
func ParseTryCloudflareURL(line string) string {
	return tryCloudflareURLPattern.FindString(line)
}

// NormalizeTunnelUpstreamURL converts a bind address like "0.0.0.0:4242" or "127.0.0.1:4242"
// into an upstream HTTP URL reachable locally by cloudflared.
func NormalizeTunnelUpstreamURL(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		trimmed = DefaultAgentServerAddress
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "0.0.0.0:") {
		trimmed = "127.0.0.1:" + strings.TrimPrefix(trimmed, "0.0.0.0:")
	} else if strings.HasPrefix(trimmed, ":") {
		trimmed = "127.0.0.1" + trimmed
	} else if strings.HasPrefix(trimmed, "[::]:") {
		trimmed = "127.0.0.1:" + strings.TrimPrefix(trimmed, "[::]:")
	}
	return "http://" + trimmed
}

// DefaultCloudflaredCacheDir returns the default directory for storing a downloaded cloudflared binary.
func DefaultCloudflaredCacheDir() string {
	if base, err := os.UserCacheDir(); err == nil && base != "" {
		return filepath.Join(base, "project-engine", "bin")
	}
	return filepath.Join(".cache", "project-engine", "bin")
}

// DefaultCloudflaredDownloadURL returns the official Cloudflare quick-tunnel binary download URL for runtime.GOOS/GOARCH.
func DefaultCloudflaredDownloadURL(goos, goarch string) (url string, isTgz bool, err error) {
	base := "https://github.com/cloudflare/cloudflared/releases/latest/download/"
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return base + "cloudflared-windows-amd64.exe", false, nil
		case "386":
			return base + "cloudflared-windows-386.exe", false, nil
		}
	case "linux":
		switch goarch {
		case "amd64", "arm64", "386", "arm":
			return base + "cloudflared-linux-" + goarch, false, nil
		}
	case "darwin":
		switch goarch {
		case "amd64", "arm64":
			return base + "cloudflared-darwin-" + goarch + ".tgz", true, nil
		}
	}
	return "", false, fmt.Errorf("tunnel: unsupported platform %s/%s for automatic cloudflared download", goos, goarch)
}

// EnsureCloudflared checks PATH and CacheDir for cloudflared, downloading the platform binary if absent.
func (m *TunnelManager) EnsureCloudflared(ctx context.Context) (string, error) {
	if strings.TrimSpace(m.cfg.BinaryPath) != "" {
		if info, err := os.Stat(m.cfg.BinaryPath); err == nil && !info.IsDir() {
			return m.cfg.BinaryPath, nil
		}
	}

	if found, err := exec.LookPath("cloudflared"); err == nil && found != "" {
		return found, nil
	}

	cacheDir := strings.TrimSpace(m.cfg.CacheDir)
	if cacheDir == "" {
		cacheDir = DefaultCloudflaredCacheDir()
	}

	exeName := "cloudflared"
	if runtime.GOOS == "windows" {
		exeName = "cloudflared.exe"
	}
	cachedPath := filepath.Join(cacheDir, exeName)
	if info, err := os.Stat(cachedPath); err == nil && !info.IsDir() && info.Size() > 0 {
		return cachedPath, nil
	}

	dlURL := strings.TrimSpace(m.cfg.DownloadURL)
	isTgz := strings.HasSuffix(strings.ToLower(dlURL), ".tgz") || strings.HasSuffix(strings.ToLower(dlURL), ".tar.gz")
	if dlURL == "" {
		var err error
		dlURL, isTgz, err = DefaultCloudflaredDownloadURL(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", err
		}
	}

	if m.console != nil {
		m.console.Print(fmt.Sprintf("[Tunnel] cloudflared not found; downloading from %s...", dlURL), LineSystem)
	}

	if info, err := os.Stat(cacheDir); err != nil || !info.IsDir() {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return "", fmt.Errorf("tunnel: create cache dir %q: %w", cacheDir, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return "", fmt.Errorf("tunnel: build download request: %w", err)
	}
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tunnel: download cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tunnel: download cloudflared failed with HTTP %d", resp.StatusCode)
	}

	tmpPath := cachedPath + ".tmp"
	if isTgz {
		if err := extractCloudflaredFromTgz(resp.Body, tmpPath); err != nil {
			return "", err
		}
	} else {
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", fmt.Errorf("tunnel: write %q: %w", tmpPath, err)
		}
		_, copyErr := io.Copy(f, resp.Body)
		closeErr := f.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("tunnel: write %q: %w", tmpPath, copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			return "", closeErr
		}
	}

	if err := os.Rename(tmpPath, cachedPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("tunnel: finalize %q: %w", cachedPath, err)
	}
	return cachedPath, nil
}

func extractCloudflaredFromTgz(r io.Reader, dstPath string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("tunnel: open tgz: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tunnel: read tgz: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && strings.HasSuffix(filepath.Base(hdr.Name), "cloudflared") {
			f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("tunnel: write %q: %w", dstPath, err)
			}
			_, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil {
				_ = os.Remove(dstPath)
				return copyErr
			}
			return closeErr
		}
	}
	return fmt.Errorf("tunnel: cloudflared binary not found in archive")
}

// Start locates/downloads cloudflared and launches an ad-hoc quick tunnel in the background,
// streaming stdout/stderr to detect the assigned trycloudflare.com URL.
func (m *TunnelManager) Start(parentCtx context.Context) error {
	binPath, err := m.EnsureCloudflared(parentCtx)
	if err != nil {
		return err
	}
	upstream := NormalizeTunnelUpstreamURL(m.cfg.TargetAddress)
	return m.StartWithCommand(parentCtx, binPath, "tunnel", "--no-autoupdate", "--url", upstream)
}

// StartWithCommand launches the specified binary and args (useful for tests as well as cloudflared)
// and monitors its stdout/stderr for the assigned quick-tunnel URL.
func (m *TunnelManager) StartWithCommand(parentCtx context.Context, binPath string, args ...string) error {
	ctx, cancel := context.WithCancel(parentCtx)
	cmd := exec.CommandContext(ctx, binPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("tunnel: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("tunnel: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("tunnel: start %q: %w", binPath, err)
	}

	done := make(chan struct{})
	m.mu.Lock()
	m.cmd = cmd
	m.cancel = cancel
	m.done = done
	m.mu.Unlock()

	if m.console != nil {
		m.console.Print(fmt.Sprintf("[Tunnel] Starting quick tunnel for %s...", NormalizeTunnelUpstreamURL(m.cfg.TargetAddress)), LineSystem)
	}

	var scanWG sync.WaitGroup
	scanPipe := func(r io.Reader) {
		defer scanWG.Done()
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := sc.Text()
			if url := ParseTryCloudflareURL(line); url != "" {
				m.recordURL(url)
			}
		}
	}
	scanWG.Add(2)
	go scanPipe(stdout)
	go scanPipe(stderr)

	go func() {
		scanWG.Wait()
		_ = cmd.Wait()
		close(done)
	}()

	return nil
}

func (m *TunnelManager) recordURL(url string) {
	m.mu.Lock()
	changed := m.url != url
	m.url = url
	onURL := m.cfg.OnURL
	m.mu.Unlock()

	if changed {
		if m.console != nil {
			m.console.Print(fmt.Sprintf("[Tunnel] Cloudflare Quick Tunnel active: %s", url), LineSystem)
		}
		if onURL != nil {
			onURL(url)
		}
	}
}

// Stop terminates the running quick-tunnel process and waits for exit.
func (m *TunnelManager) Stop() {
	if m == nil {
		return
	}
	m.mu.RLock()
	cancel := m.cancel
	done := m.done
	m.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}
