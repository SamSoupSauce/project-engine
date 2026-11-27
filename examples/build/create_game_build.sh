#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}"

export CGO_LDFLAGS_ALLOW='-l:.*\.a'

GAME_ROOT="${1:-examples/custom-game-root}"
OUT_DIR="${2:-dist/my-game}"

EXE_EXT=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) EXE_EXT=".exe" ;;
esac

if [ ! -d "${GAME_ROOT}" ]; then
  echo "Error: game root '${GAME_ROOT}' does not exist." >&2
  exit 1
fi

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

echo "[1/3] Compiling engine binary -> ${OUT_DIR}/project-engine${EXE_EXT}"
go build -o "${OUT_DIR}/project-engine${EXE_EXT}" ./cmd/demo

echo "[2/3] Copying game data root from '${GAME_ROOT}'..."
for sub in assets scripts prefabs config; do
  if [ -d "${GAME_ROOT}/${sub}" ]; then
    cp -R "${GAME_ROOT}/${sub}" "${OUT_DIR}/${sub}"
  else
    mkdir -p "${OUT_DIR}/${sub}"
  fi
done
if [ -f "${GAME_ROOT}/engine.json" ]; then
  cp "${GAME_ROOT}/engine.json" "${OUT_DIR}/engine.json"
fi

echo "[3/3] Writing standalone launcher..."
cat > "${OUT_DIR}/run_game.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
HERE="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
exec "\${HERE}/project-engine${EXE_EXT}" --root "\${HERE}" "\$@"
EOF
chmod +x "${OUT_DIR}/run_game.sh"

echo "Distributable game build created at '${OUT_DIR}'."
echo "Launch with: ./${OUT_DIR}/run_game.sh"
