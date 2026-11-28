package luabridge

import (
	"fmt"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// DocsCommandItem summarizes a native or dynamic Lua shell command for documentation queries.
type DocsCommandItem struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Usage       string   `json:"usage,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// DocsHookItem describes an engine lifecycle hook signature and semantics.
type DocsHookItem struct {
	Name        string `json:"name"`
	Signature   string `json:"signature"`
	Description string `json:"description"`
}

// DocsSubsystemItem describes a built-in engine subsystem topic.
type DocsSubsystemItem struct {
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	Usage       string   `json:"usage,omitempty"`
	Details     []string `json:"details,omitempty"`
}

// DocsQueryResult is the structured and formatted response for `docs` and `engine.query_docs`.
type DocsQueryResult struct {
	Topic          string                  `json:"topic"`
	Type           string                  `json:"type"`
	Description    string                  `json:"description,omitempty"`
	Usage          string                  `json:"usage,omitempty"`
	Source         string                  `json:"source,omitempty"`
	Examples       []string                `json:"examples,omitempty"`
	GoType         string                  `json:"go_type,omitempty"`
	Fields         []string                `json:"fields,omitempty"`
	Methods        []string                `json:"methods,omitempty"`
	Details        []string                `json:"details,omitempty"`
	Categories     []string                `json:"categories,omitempty"`
	Sections       map[string]any          `json:"sections,omitempty"`
	Subsystems     []DocsSubsystemItem     `json:"subsystems,omitempty"`
	NativeCommands []DocsCommandItem       `json:"native_commands,omitempty"`
	LuaCommands    []DocsCommandItem       `json:"lua_commands,omitempty"`
	Components     []ComponentSearchResult `json:"components,omitempty"`
	Hooks          []DocsHookItem          `json:"hooks,omitempty"`
	Formatted      string                  `json:"formatted"`
}

var engineLifecycleHooksDocs = []DocsHookItem{
	{Name: "load", Signature: "load(scene)", Description: "Called once when the owning scene initializes or when a script module is loaded via scripts.run/scripts.reload."},
	{Name: "unload", Signature: "unload(scene)", Description: "Called when the scene unloads or when scripts.unload/scripts.reload unbinds the script."},
	{Name: "tick", Signature: "tick(self, dtMs)", Description: "Fixed-rate simulation tick invoked every TickIntervalMs (default 16ms) for active spawned objects."},
	{Name: "event", Signature: "event(name, self, sender, data)", Description: "Invoked when a named event is emitted to the owning GameObject or scene event bus."},
	{Name: "spawn", Signature: "spawn(self, sender)", Description: "Called when a GameObject is spawned into an active scene."},
	{Name: "despawn", Signature: "despawn(self, sender, data)", Description: "Called immediately before a GameObject is despawned and destroyed."},
}

var builtinSubsystemDocs = map[string]DocsSubsystemItem{
	"jolt": {
		Topic:       "jolt",
		Description: "Jolt Physics LuaJIT FFI & ECS physics components (RigidBody, CharacterController, InteractiveComponent, FractureComponent).",
		Usage:       "comp add RigidBody | comp add CharacterController | comp help RigidBody",
		Details: []string{
			"RigidBody (*spatial.RigidBody): fields ID, BodyID, BodyType, Mass; methods GetPosition/SetPosition, GetVelocity/SetVelocity, AddImpulse, IsActive, IsGrounded.",
			"CharacterController (*spatial.CharacterController): capsule movement, stairs, slope limit; methods SetDesiredVelocity, Jump, SetPosition, GetPosition, IsGrounded.",
			"InteractiveComponent (*spatial.InteractiveComponent): reticle hover & quick-tap/hold verbs; methods SetDisplayName, SetDefaultVerb, AddContextVerb.",
			"FractureComponent (*spatial.FractureComponent): structural integrity & splinter/shatter debris; methods GetHealth/SetHealth, ApplyImpact, Fracture.",
		},
	},
	"shell.register": {
		Topic:       "shell.register",
		Description: "Dynamic Lua shell command registration API automatically tracked per-script and unbound on scripts.reload/scripts.unload.",
		Usage:       `shell.register("name", { description = "...", usage = "...", examples = { "..." }, handler = function(ctx, args) return true, "ok" end })`,
		Details: []string{
			"ctx fields: ctx.sender (e.g. 'user', 'agent:agy'), ctx.timestamp (RFC3339Nano), ctx.raw, ctx.args (1-based array).",
			"args: 1-based array of parsed whitespace/quote-aware command arguments (excluding command verb).",
			"Return contract: return true, outputString on success; return false, errorMessage to return a structured command error.",
			"Management helpers: shell.unregister(name) -> bool, shell.list() -> table of registered commands.",
		},
	},
	"prefab.format": {
		Topic:       "prefab.format",
		Description: "JSON GameObject Prefab specification stored under prefabs/*.json.",
		Usage:       "save_prefab([obj], name) | spawn_prefab(name, [opts]) | prefab view <name>",
		Details: []string{
			`Top-level JSON fields: "id" (optional), "name" (string), "active" (bool, defaults true), "components" (array), "children" (array of nested PrefabSpec).`,
			`Component entry schema: { "name": "Transform", "type": "*engine.TransformComponent", "details": { ... } }.`,
			"Transform details: position/rotation/scale ({x, y, z}); Mesh details: file_path; MeshRenderable: visible, wireframe.",
			"Custom LuaComponent details map directly into component instance field overrides on spawn.",
		},
	},
	"hooks": {
		Topic:       "hooks",
		Description: "In-engine GameObject & Script lifecycle hooks dispatched across ScriptComponent, module tables, and LuaComponent.",
		Usage:       "obj:CallHook(hookName, ...) | Component(name, defaults, hookSignatures)",
		Details: []string{
			"load(scene): scene or script initialization.",
			"unload(scene): scene teardown or scripts.unload cleanup.",
			"tick(self, dtMs): fixed-timestep simulation update (default 16ms).",
			"event(name, self, sender, data): scene/entity event dispatch.",
			"spawn(self, sender) & despawn(self, sender, data): entity lifecycle boundaries.",
		},
	},
	"components": {
		Topic:       "components",
		Description: "ECS Component registry covering built-in Go components and custom Lua Component(...) schemas.",
		Usage:       "comp search [query] | comp list [obj] | comp add [obj] <Type> | comp set [obj] <Type> <field> <val>",
		Details: []string{
			"Built-in classes: Transform, Mesh, MeshRenderable, Renderable, Renderable2D, RigidBody, CharacterController, InteractiveComponent, FractureComponent, ScriptComponent.",
			"Custom components: defined in scripts/components/*.lua via Component(name, defaults, hooks) and hot-reloaded via edit() or scripts.reload.",
		},
	},
	"prefabs": {
		Topic:       "prefabs",
		Description: "Prefab storage, console search, scene spawning, and ImGui Prefab Manager.",
		Usage:       "prefabs [query] | prefab save <name> [obj] | prefab spawn <name> [id] | edit_prefab([name])",
		Details: []string{
			"Storage folder: prefabs/ at repository root.",
			"Lua helpers: search_prefabs([query]), save_prefab([obj], name), load_prefab(name), spawn_prefab(name, [opts]), delete_prefab(name).",
		},
	},
	"scripts": {
		Topic:       "scripts",
		Description: "Lua script discovery, execution, hot-reloading, unloading, and tri-pane script authoring.",
		Usage:       "scripts.list [query] | scripts.run <path> [args...] | scripts.reload <name> | scripts.unload <name> | edit <name>",
		Details: []string{
			"Scripts directory: scripts/ (components/, props/, ui/).",
			"Console helpers: search_scripts([query]), scripts [query], scripts.list, scripts.run, scripts.reload, scripts.unload.",
		},
	},
	"console": {
		Topic:       "console",
		Description: "Unified command execution funnel (ExecuteWithContext) shared by Dear ImGui Console, JSON-RPC /rpc, and WebSocket /ws.",
		Usage:       "help [cmd] | docs [topic] | pick <nameOrID> | tutorial | clear",
		Details: []string{
			"All inputs route through ExecuteWithContext(ctx, rawCommand) with Sender attribution ('user' or 'agent:<id>').",
			"Remote agent invocations mirror '[agent:<id>] <cmd>' and resulting output into the in-game Console scrollback.",
		},
	},
	"workspace": {
		Topic:       "workspace",
		Description: "Configurable project root (--root, --config, engine.json), startup folder verification, and --init-dirs scaffolding.",
		Usage:       "project-engine --root <dir> --config <file> [--init-dirs] | workspace_paths()",
		Details: []string{
			"Default root is '.' with subdirectories assets/, scripts/, prefabs/, and config/ (configurable via engine.json).",
			"Startup verifies all required folders exist and fails fast with actionable instructions unless --init-dirs is passed.",
			"Lua helper workspace_paths() returns { root, assets, scripts, prefabs, config }.",
		},
	},
	"tilemap": {
		Topic:       "tilemap",
		Description: "TMX tile map discovery, runtime loading, layer/tile introspection, and the ImGui Tile Map Browser.",
		Usage:       "tilemaps [query] | tilemap.load <path> | tilemap.reload | tilemap.info | tilemap.layers | tilemap.tiles [layer] [query]",
		Details: []string{
			"Lua API: tilemap.list([query]), load(path), reload(), current(), info(), layers(), tiles([layer],[query]), tile_at(col,row,[layer]), world_to_grid(x,z), grid_to_world(col,row), colliders(), registry().",
			"Bridge RPC: engine.query_tilemaps, engine.load_tilemap ({path}), engine.query_tilemap ({topic: info|layers|tiles|tile|colliders|registry}).",
			"Bridge WS ops: query_tilemaps, load_tilemap, query_tilemap (same params as RPC).",
			"Map path is configurable via engine.json 'tilemap_path' or the --tilemap CLI flag; defaults to assets/maps/kitchen_sink.tmx.",
			"The ImGui Tile Map Browser is toggled from the [~] Developer Console; select a map to load/swap it at runtime.",
		},
	},
	"tutorial": {
		Topic:       "tutorial",
		Description: "First-run interactive Dear ImGui quick-start tutorial with persisted 'Do not show again' preference.",
		Usage:       "tutorial (console command) | config/tutorial_state.json",
		Details: []string{
			"Opens automatically on first run unless 'Do not show again' is checked and persisted in config/tutorial_state.json.",
			"Can be reopened anytime via the 'Tutorial' button in the Demo Controls window or the `tutorial` console command.",
		},
	},
}

func (b *LuaBridge) registerDocsAPIs() {
	if b.shell == nil {
		b.shell = NewShellRegistry()
	}

	b.shell.RegisterNative(ShellCommandDef{
		Name:        "docs",
		Description: "Queries dynamic runtime documentation for engine subsystems, shell commands, components, and hooks.",
		Usage:       "docs [topic|command|component]",
		Examples:    []string{"docs", "docs jolt", "docs shell.register", "docs prefab.format", "docs components"},
		GoHandler: func(ctx ExecutionContext) (string, error) {
			topic := strings.TrimSpace(strings.Join(ctx.Args[1:], " "))
			res, err := b.QueryDocs(topic)
			if err != nil {
				return "", err
			}
			b.EmitConsole(res.Formatted, LineInfo)
			return res.Formatted, nil
		},
	})

	b.L.SetGlobal("docs", b.L.NewFunction(func(L *lua.LState) int {
		topic := strings.TrimSpace(L.OptString(1, ""))
		res, err := b.QueryDocs(topic)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		b.EmitConsole(res.Formatted, LineInfo)
		L.Push(lua.LString(res.Formatted))
		return 1
	}))
}

// QueryDocs builds dynamic runtime documentation for either the full capability index (topic == "")
// or a specific shell command, ECS component, lifecycle hook, or subsystem topic.
func (b *LuaBridge) QueryDocs(topic string) (DocsQueryResult, error) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return b.buildDocsIndex(), nil
	}

	lower := strings.ToLower(trimmed)

	// 1. Check dynamically registered Lua or native shell commands first.
	if b.shell != nil {
		if def, ok := b.shell.Get(lower); ok {
			cmdType := "native_command"
			if def.IsLua {
				cmdType = "lua_command"
			}
			desc := def.Description
			if desc == "" {
				desc = fmt.Sprintf("Registered %s command %q.", cmdType, def.Name)
			}
			usage := def.Usage
			if usage == "" {
				usage = def.Name
			}
			return DocsQueryResult{
				Topic:       def.Name,
				Type:        cmdType,
				Description: desc,
				Usage:       usage,
				Source:      def.Source,
				Examples:    append([]string(nil), def.Examples...),
				Formatted:   formatShellCommandHelp(def),
			}, nil
		}
	}

	// 2. Check registered subsystem topics (jolt, shell.register, prefab.format, hooks, components, etc.).
	if sub, ok := builtinSubsystemDocs[lower]; ok {
		var formatted strings.Builder
		formatted.WriteString(fmt.Sprintf("=== Subsystem Docs: %s ===\n", sub.Topic))
		formatted.WriteString(fmt.Sprintf("Description: %s\n", sub.Description))
		if sub.Usage != "" {
			formatted.WriteString(fmt.Sprintf("Usage:       %s\n", sub.Usage))
		}
		if len(sub.Details) > 0 {
			formatted.WriteString("Details:\n")
			for _, d := range sub.Details {
				formatted.WriteString("  - " + d + "\n")
			}
		}
		var hooks []DocsHookItem
		var comps []ComponentSearchResult
		if lower == "hooks" {
			hooks = append([]DocsHookItem(nil), engineLifecycleHooksDocs...)
		}
		if lower == "components" {
			comps = b.SearchComponents("")
		}
		return DocsQueryResult{
			Topic:       sub.Topic,
			Type:        "subsystem",
			Description: sub.Description,
			Usage:       sub.Usage,
			Source:      "engine",
			Details:     append([]string(nil), sub.Details...),
			Hooks:       hooks,
			Components:  comps,
			Formatted:   strings.TrimRight(formatted.String(), "\n"),
		}, nil
	}

	// 3. Check registered ECS components (built-in and custom Lua components).
	for _, comp := range b.SearchComponents("") {
		if strings.EqualFold(comp.Name, trimmed) || strings.EqualFold(resolveComponentAlias(trimmed), comp.Name) {
			txt, _ := b.formatNamedTypeHelp(comp.Name)
			src := comp.SourcePath
			if src == "" {
				src = "builtin"
			}
			return DocsQueryResult{
				Topic:       comp.Name,
				Type:        "component",
				Description: comp.Description,
				GoType:      comp.GoType,
				Fields:      append([]string(nil), comp.Fields...),
				Methods:     append([]string(nil), comp.Methods...),
				Source:      src,
				Formatted:   txt,
			}, nil
		}
	}

	// 4. Check individual lifecycle hook name (e.g. `docs tick`, `docs spawn`).
	for _, hk := range engineLifecycleHooksDocs {
		if strings.EqualFold(hk.Name, trimmed) {
			txt := fmt.Sprintf("=== Hook Docs: %s ===\nSignature:   %s\nDescription: %s", hk.Name, hk.Signature, hk.Description)
			return DocsQueryResult{
				Topic:       hk.Name,
				Type:        "hook",
				Description: hk.Description,
				Usage:       hk.Signature,
				Source:      "engine",
				Formatted:   txt,
			}, nil
		}
	}

	return DocsQueryResult{}, fmt.Errorf("documentation topic %q not found (run 'docs' for index)", trimmed)
}

func (b *LuaBridge) buildDocsIndex() DocsQueryResult {
	subKeys := make([]string, 0, len(builtinSubsystemDocs))
	for k := range builtinSubsystemDocs {
		subKeys = append(subKeys, k)
	}
	sort.Strings(subKeys)
	subsystems := make([]DocsSubsystemItem, 0, len(subKeys))
	for _, k := range subKeys {
		subsystems = append(subsystems, builtinSubsystemDocs[k])
	}

	var nativeCmds, luaCmds []DocsCommandItem
	if b.shell != nil {
		for _, sc := range b.shell.List() {
			item := DocsCommandItem{
				Name:        sc.Name,
				Description: sc.Description,
				Usage:       sc.Usage,
				Examples:    append([]string(nil), sc.Examples...),
				Source:      sc.Source,
			}
			if sc.IsLua {
				item.Type = "lua_command"
				luaCmds = append(luaCmds, item)
			} else {
				item.Type = "native_command"
				nativeCmds = append(nativeCmds, item)
			}
		}
	}

	comps := b.SearchComponents("")
	hooks := append([]DocsHookItem(nil), engineLifecycleHooksDocs...)

	var sb strings.Builder
	sb.WriteString("=== Project Engine Runtime Documentation Index ===\n")
	sb.WriteString("Use `docs <topic|command|component>` for detailed usage and schemas.\n\n")

	sb.WriteString("Subsystems:\n")
	for _, s := range subsystems {
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", s.Topic, s.Description))
	}

	sb.WriteString(fmt.Sprintf("\nNative Shell Commands (%d):\n", len(nativeCmds)))
	for _, c := range nativeCmds {
		if c.Description != "" {
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", c.Name, c.Description))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", c.Name))
		}
	}

	sb.WriteString(fmt.Sprintf("\nDynamic Lua Shell Commands (%d):\n", len(luaCmds)))
	if len(luaCmds) == 0 {
		sb.WriteString("  (none currently loaded; run `scripts.run <path>` to load script commands)\n")
	} else {
		for _, c := range luaCmds {
			sb.WriteString(fmt.Sprintf("  %-16s %s [%s]\n", c.Name, c.Description, c.Source))
		}
	}

	sb.WriteString(fmt.Sprintf("\nECS Components (%d):\n", len(comps)))
	for _, comp := range comps {
		tag := "builtin"
		if comp.Custom {
			tag = "custom"
		}
		sb.WriteString(fmt.Sprintf("  %-20s [%s] %s\n", comp.Name, tag, comp.Description))
	}

	sb.WriteString(fmt.Sprintf("\nLifecycle Hooks (%d):\n", len(hooks)))
	for _, h := range hooks {
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", h.Signature, h.Description))
	}

	return DocsQueryResult{
		Topic:          "index",
		Type:           "index",
		Description:    "Runtime index of engine subsystems, native & dynamic Lua shell commands, ECS components, and lifecycle hooks.",
		Categories:     []string{"subsystems", "native_commands", "lua_commands", "components", "hooks"},
		Subsystems:     subsystems,
		NativeCommands: nativeCmds,
		LuaCommands:    luaCmds,
		Components:     comps,
		Hooks:          hooks,
		Sections: map[string]any{
			"subsystems":      subsystems,
			"native_commands": nativeCmds,
			"lua_commands":    luaCmds,
			"components":      comps,
			"hooks":           hooks,
		},
		Formatted: strings.TrimRight(sb.String(), "\n"),
	}
}
