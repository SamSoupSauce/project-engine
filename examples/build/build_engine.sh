#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}"

# Required by Go CGO to link bundled static archives (libjoltc.a / libluajit.a)
export CGO_LDFLAGS_ALLOW='-l:.*\.a'

EXE_EXT=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) EXE_EXT=".exe" ;;
esac
OUTPUT="${1:-bin/project-engine${EXE_EXT}}"

mkdir -p "$(dirname "${OUTPUT}")"
echo "[1/2] Verifying Go toolchain..."
go version

echo "[2/2] Building Project Engine executable -> ${OUTPUT}"
go build -o "${OUTPUT}" ./cmd/demo

echo "Build succeeded: ${OUTPUT}"
echo "Run locally:  ./${OUTPUT}"
echo "Custom root:  ./${OUTPUT} --root examples/custom-game-root"
