package luabridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestTicket18CommitNewComponentScriptExistingAndNewDir(t *testing.T) {
	for _, precreateComponentsDir := range []bool{true, false} {
		name := "new_dir"
		if precreateComponentsDir {
			name = "existing_dir"
		}
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if precreateComponentsDir {
				if err := os.MkdirAll(filepath.Join(tmpDir, "components"), 0o755); err != nil {
					t.Fatalf("mkdir components failed: %v", err)
				}
			}

			bridge := NewLuaBridge()
			defer bridge.Close()
			bridge.ScriptEditor().SetScriptsDir(tmpDir)
			console := NewConsole(bridge)

			overlay := engine.NewImGuiOverlay()
			overlay.InitHeadless(1280, 720)
			defer overlay.Shutdown()
			overlay.SetConsole(console)
			overlay.SetScriptEditor(bridge.ScriptEditor())

			// Reproduce console workflow: define CC, open edit("CC"), author DoIt(), and Commit.
			console.ExecuteCommand(`
				CC = Component("CC", {}, { DoIt = {} })
				function CC:DoIt()
					print("Did It")
				end
				edit("CC")
			`)

			overlay.NewFrame()
			overlay.DrawDebugWindows()
			overlay.Render()

			edWin := overlay.ScriptEditor
			if !edWin.IsOpen {
				t.Fatalf("expected ScriptEditor open for CC")
			}
			edWin.Doc.FunctionsLua = `function CC:DoIt()
  print("Did It")
end
`
			if err := edWin.Commit(); err != nil {
				t.Fatalf("Commit failed: %v", err)
			}
			if edWin.StatusIsErr {
				t.Fatalf("expected clean commit status, got error: %s", edWin.StatusMsg)
			}

			targetPath := filepath.Join(tmpDir, "components", "CC.lua")
			if _, err := os.Stat(targetPath); err != nil {
				t.Fatalf("expected %q to exist on disk: %v", targetPath, err)
			}

			// Verify attached instance invokes committed CC:DoIt().
			obj := engine.NewGameObject("cc_obj", "CCObj")
			bridge.RegisterGameObject("cc_obj", obj)
			console.PickObject(obj)
			res := console.ExecuteCommand(`
				selected:AddComponent(CC)
				selected:CallHook("DoIt")
			`)
			if !strings.Contains(res, "Did It") {
				t.Fatalf("expected CallHook('DoIt') output to contain 'Did It', got %q", res)
			}
		})
	}
}
