#!/bin/bash
# Cross-builds the vidpolish Windows GUI (cmd/vidpolish-gui) + CLI and
# packages them into a single NSIS installer, using mingw-w64 to
# cross-compile the cgo-based GUI binary from Linux. See
# scripts/install-gui-toolchain.sh for the one-time toolchain setup this
# depends on (wails CLI, mingw-w64, nsis).
#
# Usage: scripts/build-gui-windows.sh [version]
#   version defaults to the contents of ./VERSION (plain semver, no "v").
#
# Output: cmd/vidpolish-gui/build/bin/vidpolish-amd64-installer.exe, also
# copied to ~/Downloads/vidpolish-setup-<version>.exe for easy grabbing
# from Windows via WSL2 interop.
set -euo pipefail

cd "$(dirname "$0")/.."

RAW_VERSION="${1:-$(cat VERSION)}"

command -v wails >/dev/null 2>&1 || { echo "error: wails CLI not found; run scripts/install-gui-toolchain.sh first" >&2; exit 1; }
command -v makensis >/dev/null 2>&1 || { echo "error: makensis not found; run scripts/install-gui-toolchain.sh first" >&2; exit 1; }
command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 || { echo "error: x86_64-w64-mingw32-gcc not found; run scripts/install-gui-toolchain.sh first" >&2; exit 1; }

echo "==> Building frontend (web/dist -> internal/server/webdist)"
(cd web && npm ci && npm run build)

GUI_DIR="cmd/vidpolish-gui"
BIN_DIR="${GUI_DIR}/build/bin"
mkdir -p "$BIN_DIR"

echo "==> Building CLI (windows/amd64, staged into the installer)"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags "-s -w -X main.version=${RAW_VERSION}" \
    -o "${BIN_DIR}/vidpolish.exe" \
    ./cmd/vidpolish

echo "==> Setting installer product version to ${RAW_VERSION}"
tmp_wails_json="$(mktemp)"
sed "s/\"productVersion\": \"[^\"]*\"/\"productVersion\": \"${RAW_VERSION}\"/" \
    "${GUI_DIR}/wails.json" > "$tmp_wails_json"
mv "$tmp_wails_json" "${GUI_DIR}/wails.json"

echo "==> Cross-compiling GUI (windows/amd64) and packaging the NSIS installer"
(
    cd "$GUI_DIR"
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        wails build -platform windows/amd64 -ldflags "-X main.version=${RAW_VERSION}" -nsis -devtools
)

INSTALLER="${BIN_DIR}/vidpolish-amd64-installer.exe"
if [ ! -f "$INSTALLER" ]; then
    echo "error: expected installer not found at ${INSTALLER}" >&2
    exit 1
fi

DEST="$HOME/Downloads/vidpolish-setup-${RAW_VERSION}.exe"
mkdir -p "$HOME/Downloads"
cp "$INSTALLER" "$DEST"

echo "==> Done: ${DEST}"
echo "    (grab it from Windows via \\\\wsl\$\\ or your distro's file share, then run it)"
