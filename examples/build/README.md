# Building the Engine & Creating a Game Build

This folder contains concrete scripts for both workflows:

1. **Building the Engine Executable** (`build_engine.ps1` / `build_engine.sh`):
   Compiles `./cmd/demo` into `bin/project-engine(.exe)` with `CGO_LDFLAGS_ALLOW='-l:.*\.a'` so bundled static libraries (`libjoltc.a`, `libluajit.a`) link cleanly.

2. **Creating a Distributable Game Build** (`create_game_build.ps1` / `create_game_build.sh`):
   Compiles the engine executable and packages it alongside an external game root (by default `examples/custom-game-root/`) into `dist/my-game/` with a self-contained launcher (`run_game.ps1` / `run_game.sh`).

## Quick Start

### Windows (PowerShell)
```powershell
# 1. Build the engine executable (or run root .\build.ps1)
.\examples\build\build_engine.ps1

# 2. Package a standalone game build from examples/custom-game-root
.\examples\build\create_game_build.ps1
```

### Linux / macOS / Git Bash
```bash
# 1. Build the engine executable (or run `make` / `./build.sh`)
./examples/build/build_engine.sh

# 2. Package a standalone game build from examples/custom-game-root
./examples/build/create_game_build.sh
```
