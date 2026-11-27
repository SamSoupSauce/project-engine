#!/usr/bin/env bash
set -euo pipefail

export CGO_LDFLAGS_ALLOW='-l:.*\.a'

EXE_EXT=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) EXE_EXT=".exe" ;;
esac

OUT_DIR="${OUT_DIR:-bin}"
DIST_DIR="${DIST_DIR:-dist/project-engine}"
VERSION="${VERSION:-0.1.0}"
TARGET="${OUT_DIR}/project-engine${EXE_EXT}"
LDFLAGS="-X github.com/SamSoupSauce/project-engine/engine.Version=${VERSION}"

mkdir -p "${OUT_DIR}"
echo "Building engine executable -> ${TARGET} (version ${VERSION})"
go build -ldflags "${LDFLAGS}" -o "${TARGET}" ./cmd/demo

for arg in "$@"; do
  case "$arg" in
    --test)
      echo "Running Go test suite..."
      go test -count=1 ./...
      ;;
    --package)
      echo "Creating standalone game build -> ${DIST_DIR}"
      rm -rf "${DIST_DIR}"
      mkdir -p "${DIST_DIR}/config"
      cp "${TARGET}" "${DIST_DIR}/project-engine${EXE_EXT}"
      cp engine.json "${DIST_DIR}/engine.json"
      cp -R assets scripts prefabs "${DIST_DIR}/"
      echo "Standalone build ready at ${DIST_DIR}"
      ;;
  esac
done
