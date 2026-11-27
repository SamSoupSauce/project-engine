# Building, Packaging & Distribution Guide

## 1. Prerequisites

- **Go**: `1.25.0` or newer
- **C/C++ Toolchain (CGO)**:
  - **Windows**: `gcc` / `g++` (MinGW-w64 / MSYS2) in `PATH`
  - **Linux**: `gcc`, `g++`, and X11/OpenGL development headers (`libgl1-mesa-dev`, `xorg-dev`)
  - **macOS**: Xcode Command Line Tools (`clang` / `clang++`)
- **CGO Static Archive Flag**:
  Because `engine/spatial` and `pkg/luabridge` link bundled static archives (`libjoltc.a` and `libluajit.a`), set:
  ```bash
  export CGO_LDFLAGS_ALLOW='-l:.*\.a'
  ```
  (The provided `Makefile`, `build.ps1`, `build.sh`, and `examples/build/*` scripts set this automatically.)

---

## 2. Default Engine Build Target

The repository root provides a default build target producing `bin/project-engine.exe` (Windows) or `bin/project-engine` (Linux/macOS):

- **Make**:
  ```bash
  make        # runs default `build` target -> bin/project-engine(.exe)
  make test   # runs full Go test suite
  make run    # builds and launches bin/project-engine(.exe)
  ```
- **Windows PowerShell**:
  ```powershell
  .\build.ps1          # builds bin/project-engine.exe
  .\build.ps1 -Test    # builds and runs go test ./...
  ```
- **Bash**:
  ```bash
  ./build.sh           # builds bin/project-engine
  ./build.sh --test    # builds and runs go test ./...
  ```

---

## 3. Creating a Distributable Game Build

A Project Engine game distribution pairs the compiled `project-engine(.exe)` binary with a self-contained workspace root (`engine.json`, `assets/`, `scripts/`, `prefabs/`, `config/`):

```powershell
# Using root build script (packages the repo's kitchen-sink demo into dist/project-engine/)
.\build.ps1 -Package
# or
make package

# Using the standalone game-build example (packages examples/custom-game-root into dist/my-game/)
.\examples\build\create_game_build.ps1
# or
./examples/build/create_game_build.sh
```

Launch the packaged game directly via its generated launcher (`./dist/my-game/run_game.sh` or `.\dist\my-game\run_game.ps1`), which passes `--root <build_dir>`.

---

## 4. Custom Root Directory & Folder Verification

### Command-Line Options
- `--root <dir>` — Root directory containing `assets/`, `scripts/`, `prefabs/`, `config/`, and optional `engine.json` (defaults to `.`).
- `--config <file>` — Path to a JSON workspace config file. If `--root` is omitted, the root defaults to the config file's parent directory (or its `root_dir` field).
- `--init-dirs` — Creates missing standard directories (`assets/models/props`, `assets/maps`, `scripts/components`, `scripts/props`, `scripts/ui`, `prefabs`, `config`) before startup verification.

### `engine.json` Schema
```json
{
  "root_dir": ".",
  "assets_dir": "assets",
  "scripts_dir": "scripts",
  "prefabs_dir": "prefabs",
  "config_dir": "config",
  "window_title": "My Custom Game"
}
```
All directory fields may be relative to `root_dir` or absolute paths.

### Startup Folder Verification
On boot, `engine.VerifyWorkspaceStructure` checks that `root`, `assets`, `scripts`, `prefabs`, and `config` exist as directories. If any are missing, the engine exits with a clear diagnostic listing every missing path and instructing you to pass `--root <dir>` or `--init-dirs`.

---

## 5. Multi-Platform CI & GitHub Releases

The GitHub Actions workflow [`.github/workflows/ci-release.yml`](../.github/workflows/ci-release.yml) builds and packages standalone distributions across 6 platform targets:
- **macOS:** `darwin/arm64` (Apple Silicon) & `darwin/amd64` (Intel)
- **Windows:** `windows/amd64` (x64) & `windows/arm64` (ARM64)
- **Ubuntu LTS:** `linux/amd64` (x64) & `linux/arm64` (ARM64)

Pushing a version tag (`git tag -a v0.1.0 -m "Release v0.1.0" && git push origin v0.1.0`) automatically runs the matrix and attaches all 6 packaged archives to the GitHub Release.
