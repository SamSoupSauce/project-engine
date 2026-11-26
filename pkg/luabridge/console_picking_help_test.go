package luabridge

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

func TestConsoleHelpFunctionPrintAndStdioRedirection(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)
	console.TeeStdio = false

	// 1. help() callable as Lua function (both help() and help('pick')) and CLI command `help`.
	outFn := console.ExecuteCommand("help()")
	if !strings.Contains(outFn, "Project Engine Developer Console Help") || !strings.Contains(outFn, "pick(") {
		t.Fatalf("expected structured help() output, got %q", outFn)
	}
	outTopic := console.ExecuteCommand("help('pick')")
	if !strings.Contains(outTopic, "Object Picking Help") {
		t.Fatalf("expected topic-filtered help('pick'), got %q", outTopic)
	}
	outCmd := console.ExecuteCommand("help")
	if !strings.Contains(outCmd, "Console Commands:") {
		t.Fatalf("expected bare `help` command help output, got %q", outCmd)
	}

	// 2. Lua print(...) and io.write(...) forward directly into console output.
	console.ClearOutput()
	console.ExecuteCommand(`print("hello", 99); io.write("io_line\n")`)
	lines := console.GetOutputLines()
	var foundPrint, foundIO bool
	for _, l := range lines {
		if strings.Contains(l.Text, "hello\t99") && l.Kind == engine.ConsoleLineOutput {
			foundPrint = true
		}
		if strings.Contains(l.Text, "io_line") && l.Kind == engine.ConsoleLineOutput {
			foundIO = true
		}
	}
	if !foundPrint || !foundIO {
		t.Fatalf("expected Lua print & io.write in console output, got %+v", lines)
	}

	// 3. Process os.Stdout, os.Stderr, and log output redirect cleanly and restore without deadlock.
	origOut := os.Stdout
	origErr := os.Stderr
	console.ClearOutput()
	if err := console.RedirectStdio(); err != nil {
		t.Fatalf("RedirectStdio failed: %v", err)
	}
	fmt.Println("stdout_probe_msg")
	fmt.Fprintln(os.Stderr, "stderr_probe_msg")
	log.Println("stdlog_probe_msg")
	console.RestoreStdio()

	if os.Stdout != origOut || os.Stderr != origErr {
		t.Fatalf("expected RestoreStdio to restore os.Stdout and os.Stderr")
	}

	captured := console.GetOutputLines()
	var gotOut, gotErr, gotLog bool
	for _, l := range captured {
		if l.Text == "stdout_probe_msg" && l.Kind == engine.ConsoleLineOutput {
			gotOut = true
		}
		if l.Text == "stderr_probe_msg" && l.Kind == engine.ConsoleLineError {
			gotErr = true
		}
		if strings.Contains(l.Text, "stdlog_probe_msg") && l.Kind == engine.ConsoleLineError {
			gotLog = true
		}
	}
	if !gotOut || !gotErr || !gotLog {
		t.Fatalf("missing redirected stdio lines (out=%v err=%v log=%v): %+v", gotOut, gotErr, gotLog, captured)
	}
}

func TestConsoleObjectPickingAndLuaSelection(t *testing.T) {
	bridge := NewLuaBridge()
	defer bridge.Close()
	console := NewConsole(bridge)

	scene := engine.NewBaseScene("pick_lua")
	scene.Camera = engine.Camera3D{
		Position:   engine.NewVector3(0, 0, 4),
		Target:     engine.NewVector3(0, 0, 0),
		Up:         engine.NewVector3(0, 1, 0),
		Fovy:       50,
		Projection: engine.CameraPerspective,
	}

	cube := engine.NewGameObject("rainbow_cube", "RainbowCube")
	cube.Transform.Position = engine.NewVector3(0, 0, 0)
	scene.AddObject(cube)

	console.BindScene(scene)

	// Screen center raycast picks RainbowCube and binds Lua globals `selected` / `picked`.
	res := console.PickAtScreenPos(scene.Camera, 640, 360, 1280, 720)
	if !res.Hit || res.Object != cube {
		t.Fatalf("expected center screen pick to hit rainbow_cube, got %+v", res)
	}
	id, name, active := console.GetPickedSummary()
	if !active || id != "rainbow_cube" || name != "RainbowCube" {
		t.Fatalf("unexpected GetPickedSummary: active=%v id=%q name=%q", active, id, name)
	}

	// Mutate selected object directly from Lua console (`selected` global).
	console.ExecuteCommand(`selected:SetPosition(9, 8, 7)`)
	if cube.Transform.Position.X != 9 || cube.Transform.Position.Y != 8 || cube.Transform.Position.Z != 7 {
		t.Fatalf("expected selected:SetPosition to mutate picked cube, got %+v", cube.Transform.Position)
	}

	// list_objects() & unpick() & pick(name) from Lua.
	listOut := console.ExecuteCommand(`return #list_objects()`)
	if listOut != "1" {
		t.Fatalf("expected 1 scene object in list_objects(), got %q", listOut)
	}
	console.ExecuteCommand(`unpick()`)
	if _, _, stillActive := console.GetPickedSummary(); stillActive {
		t.Fatalf("expected selection cleared after unpick()")
	}
	pickedName := console.ExecuteCommand(`pick("RainbowCube"); return picked():GetID()`)
	if pickedName != "rainbow_cube" {
		t.Fatalf("expected pick('RainbowCube') to bind picked(), got %q", pickedName)
	}
}
