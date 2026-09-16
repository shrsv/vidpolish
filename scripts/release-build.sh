#!/bin/bash
# Cross-compile vidpolish release binaries for all supported platforms.
#
# Usage: scripts/release-build.sh [version]
#   version defaults to the contents of ./VERSION (no "v" prefix, plain
#   semver like "0.1.0"); the "v" prefix is added here for tag/dir naming.
#
# Output: dist/v<version>/vidpolish-<os>-<arch>[.exe] plus checksums.txt.
set -euo pipefail

cd "$(dirname "$0")/.."

RAW_VERSION="${1:-$(cat VERSION)}"
if ! [[ "$RAW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$ ]]; then
    echo "error: version '$RAW_VERSION' doesn't look like semver (X.Y.Z or X.Y.Z-suffix)" >&2
    exit 1
fi
VERSION="v${RAW_VERSION}"

echo "==> Building vidpolish ${VERSION}"

echo "==> Building frontend (web/dist -> internal/server/webdist)"
(cd web && npm ci && npm run build)

if [ ! -d internal/server/webdist/assets ]; then
    echo "error: internal/server/webdist/assets missing after web build; embed will be stale" >&2
    exit 1
fi

OUT_DIR="dist/${VERSION}"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# os/arch pairs vidpolish's own dependency fetchers (internal/binmgr) can
# actually support (see internal/binmgr/assets.go).
PLATFORMS=(
    "linux amd64"
    "linux arm64"
    "darwin amd64"
    "darwin arm64"
    "windows amd64"
)

for platform in "${PLATFORMS[@]}"; do
    read -r GOOS GOARCH <<< "$platform"
    ext=""
    if [ "$GOOS" = "windows" ]; then
        ext=".exe"
    fi
    out="${OUT_DIR}/vidpolish-${GOOS}-${GOARCH}${ext}"
    echo "==> Building ${GOOS}/${GOARCH} -> ${out}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
        -ldflags "-s -w -X main.version=${RAW_VERSION}" \
        -o "$out" \
        ./cmd/vidpolish
done

echo "==> Building Windows GUI + NSIS installer"
if ! command -v wails >/dev/null 2>&1 || ! command -v makensis >/dev/null 2>&1 || ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    echo "error: wails/makensis/x86_64-w64-mingw32-gcc not found; run scripts/install-gui-toolchain.sh first" >&2
    exit 1
fi

GUI_DIR="cmd/vidpolish-gui"
GUI_BIN_DIR="${GUI_DIR}/build/bin"
mkdir -p "$GUI_BIN_DIR"
cp "${OUT_DIR}/vidpolish-windows-amd64.exe" "${GUI_BIN_DIR}/vidpolish.exe"

tmp_wails_json="$(mktemp)"
sed "s/\"productVersion\": \"[^\"]*\"/\"productVersion\": \"${RAW_VERSION}\"/" \
    "${GUI_DIR}/wails.json" > "$tmp_wails_json"
mv "$tmp_wails_json" "${GUI_DIR}/wails.json"

(
    cd "$GUI_DIR"
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        wails build -platform windows/amd64 -ldflags "-X main.version=${RAW_VERSION}" -nsis -devtools
)

cp "${GUI_BIN_DIR}/vidpolish-amd64-installer.exe" "${OUT_DIR}/vidpolish-setup-windows-amd64.exe"

echo "==> Writing checksums"
(cd "$OUT_DIR" && sha256sum vidpolish-* > checksums.txt)

echo "==> Done. Artifacts in ${OUT_DIR}:"
ls -la "$OUT_DIR"
