#!/bin/bash
# Installs/verifies the toolchain needed to build and package the vidpolish
# Windows GUI (cmd/vidpolish-gui) from a Linux dev machine: the Wails CLI,
# mingw-w64 (cross-compiling the cgo-based GUI binary for Windows), and NSIS
# (packaging the installer). Versions come from versions.env so there's one
# place to bump them.
#
# Usage: scripts/install-gui-toolchain.sh
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=/dev/null
source versions.env

echo "==> Installing Wails CLI ${WAILS_VERSION}"
go install "github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}"

need_apt=()
if ! command -v makensis >/dev/null 2>&1; then
    need_apt+=("nsis")
fi
if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    need_apt+=("gcc-mingw-w64-x86-64")
fi

if [ "${#need_apt[@]}" -gt 0 ]; then
    echo "==> Missing: ${need_apt[*]}"
    echo "    Install with:"
    echo "      sudo apt update && sudo apt install -y ${need_apt[*]}"
    echo "    (expected versions: nsis=${NSIS_VERSION}, mingw-w64 gcc=${MINGW_VERSION})"
else
    echo "==> OK: makensis and x86_64-w64-mingw32-gcc already present"
fi

echo ""
echo "==> Versions found:"
command -v wails >/dev/null 2>&1 && wails version || echo "wails: not on PATH (check GOPATH/bin is on PATH)"
command -v makensis >/dev/null 2>&1 && makensis /VERSION || true
command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 && x86_64-w64-mingw32-gcc --version | head -1 || true
