#!/bin/bash
# vidpolish installer - downloads and installs the latest (or a pinned)
# vidpolish release binary.
# Usage: curl -fsSL https://raw.githubusercontent.com/shrsv/vidpolish/main/scripts/install.sh | bash
#
# - Installs to ~/.local/bin (user-writable, no sudo required).
# - Sets up PATH via an idempotent ~/.vidpolish/env shim sourced from shell
#   rc files, so the binary needs no shell restart to be usable.
# - Runs `vidpolish deps` after install so ffmpeg is checked and
#   deep-filter/auto-editor/resvg/fonts get fetched immediately, with live
#   progress (vidpolish itself prints percentage/speed/ETA per tool).
# - Set VIDPOLISH_VERSION=vX.Y.Z to pin a version instead of installing the
#   latest GitHub release.
# - Set VIDPOLISH_SKIP_DEPS=1 to skip the post-install `vidpolish deps` run.
#
# Every network call below has an explicit timeout and prints a visible
# progress indicator, so the script never just sits there with no output —
# if something looks stuck for more than the printed timeout, it has
# actually failed and you'll see why.

set -e

REPO="shrsv/vidpolish"
INSTALL_DIR="$HOME/.local/bin"
INSTALL_PATH="$INSTALL_DIR/vidpolish"
VIDPOLISH_SKIP_DEPS="${VIDPOLISH_SKIP_DEPS:-0}"
TOTAL_STEPS=5

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

step() {
    echo ""
    echo -e "${BOLD}[$1/${TOTAL_STEPS}] $2${NC}"
}

fail() {
    echo -e "${RED}Error: $1${NC}" >&2
    if [ -n "${2:-}" ]; then
        echo -e "${YELLOW}$2${NC}" >&2
    fi
    exit 1
}

echo "vidpolish installer"
echo "===================="

# ---------------------------------------------------------------------------
# Step 1: detect platform
# ---------------------------------------------------------------------------
step 1 "Detecting platform"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux*)
        PLATFORM_OS="linux"
        ;;
    darwin*)
        PLATFORM_OS="darwin"
        ;;
    msys*|mingw*|cygwin*)
        echo -e "${YELLOW}Windows shell detected.${NC}"
        echo "This installer targets Linux/macOS. On Windows, download"
        echo "vidpolish-windows-amd64.exe directly from:"
        echo "  https://github.com/${REPO}/releases/latest"
        exit 1
        ;;
    *)
        fail "unsupported operating system: $OS"
        ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)
        PLATFORM_ARCH="amd64"
        ;;
    aarch64|arm64)
        PLATFORM_ARCH="arm64"
        ;;
    *)
        fail "unsupported architecture: $ARCH"
        ;;
esac

PLATFORM="${PLATFORM_OS}-${PLATFORM_ARCH}"
echo -e "${GREEN}OK${NC} platform: ${PLATFORM}"

# ---------------------------------------------------------------------------
# Step 2: resolve version
# ---------------------------------------------------------------------------
step 2 "Resolving version"

if [ -n "${VIDPOLISH_VERSION:-}" ]; then
    TAG="$VIDPOLISH_VERSION"
    echo -e "${GREEN}OK${NC} using pinned version: ${TAG}"
else
    echo "Querying GitHub for the latest release (timeout 15s)..."
    LATEST_JSON=$(curl --connect-timeout 10 --max-time 15 -fsSL "https://api.github.com/repos/${REPO}/releases/latest") \
        || fail "failed to query GitHub for the latest release" \
                "Check your internet connection and try again, or set VIDPOLISH_VERSION=vX.Y.Z to skip this lookup."
    TAG=$(echo "$LATEST_JSON" | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    if [ -z "$TAG" ]; then
        fail "could not parse tag_name from GitHub's response" \
             "See https://github.com/${REPO}/releases/latest and pass VIDPOLISH_VERSION=<tag> instead."
    fi
    echo -e "${GREEN}OK${NC} latest version: ${TAG}"
fi

BINARY_NAME="vidpolish-${PLATFORM}"
FULL_URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY_NAME}"

# ---------------------------------------------------------------------------
# Step 3: download
# ---------------------------------------------------------------------------
step 3 "Downloading ${BINARY_NAME} (${TAG})"
echo "From: ${FULL_URL}"

mkdir -p "$INSTALL_DIR"
TMP_FILE=$(mktemp)
trap 'rm -f "$TMP_FILE"' EXIT

# No -s here on purpose: curl's own progress meter (transfer size, speed,
# ETA) prints live to the terminal so a slow connection is visibly
# progressing rather than looking hung. -w captures the HTTP status
# separately on stdout, after the meter has finished.
HTTP_CODE=$(curl -L --connect-timeout 15 --max-time 600 \
    -w "%{http_code}" -o "$TMP_FILE" "$FULL_URL") \
    || fail "download failed (network error or timeout after 10 minutes)" \
            "Check your internet connection and try again, or download ${BINARY_NAME} manually from https://github.com/${REPO}/releases/tag/${TAG}"

if [ "$HTTP_CODE" != "200" ]; then
    fail "download failed (HTTP $HTTP_CODE) from $FULL_URL" \
         "That platform/version combination may not have a published binary; check https://github.com/${REPO}/releases/tag/${TAG}"
fi
if [ ! -s "$TMP_FILE" ]; then
    fail "downloaded file is empty"
fi
echo -e "${GREEN}OK${NC} downloaded $(du -h "$TMP_FILE" | cut -f1)"

# ---------------------------------------------------------------------------
# Step 4: install + PATH setup
# ---------------------------------------------------------------------------
step 4 "Installing to ${INSTALL_PATH} and setting up PATH"

mv "$TMP_FILE" "$INSTALL_PATH" || fail "failed to install to ${INSTALL_PATH}"
trap - EXIT
chmod +x "$INSTALL_PATH"
echo -e "${GREEN}OK${NC} installed"

if [ "$PLATFORM_OS" = "darwin" ]; then
    xattr -d com.apple.quarantine "$INSTALL_PATH" 2>/dev/null || true
fi

VIDPOLISH_ENV_DIR="$HOME/.vidpolish"
VIDPOLISH_ENV_FILE="$VIDPOLISH_ENV_DIR/env"

mkdir -p "$VIDPOLISH_ENV_DIR"
cat > "$VIDPOLISH_ENV_FILE" << 'ENVEOF'
#!/bin/sh
# vidpolish shell setup (auto-generated by the vidpolish installer)
case ":${PATH}:" in
    *:"$HOME/.local/bin":*)
        ;;
    *)
        export PATH="$HOME/.local/bin:$PATH"
        ;;
esac
ENVEOF
chmod +x "$VIDPOLISH_ENV_FILE"

add_source_line() {
    local rcfile="$1"
    if [ -f "$rcfile" ] && [ -r "$rcfile" ]; then
        if ! grep -qF '/.vidpolish/env' "$rcfile"; then
            echo "" >> "$rcfile"
            echo "# Added by vidpolish installer" >> "$rcfile"
            echo ". \"\$HOME/.vidpolish/env\"" >> "$rcfile"
            echo -e "${GREEN}OK${NC} updated $rcfile"
        fi
    fi
}

create_source_line() {
    local rcfile="$1"
    if [ ! -f "$rcfile" ]; then
        echo "# Added by vidpolish installer" > "$rcfile"
        echo ". \"\$HOME/.vidpolish/env\"" >> "$rcfile"
        echo -e "${GREEN}OK${NC} created $rcfile"
    else
        add_source_line "$rcfile"
    fi
}

create_source_line "$HOME/.profile"

CURRENT_SHELL="$(basename "${SHELL:-/bin/sh}")"
case "$CURRENT_SHELL" in
    bash)
        add_source_line "$HOME/.bashrc"
        add_source_line "$HOME/.bash_profile"
        ;;
    zsh)
        create_source_line "$HOME/.zshenv"
        add_source_line "$HOME/.zshrc"
        ;;
    fish)
        FISH_CONF_DIR="$HOME/.config/fish/conf.d"
        FISH_CONF="$FISH_CONF_DIR/vidpolish.fish"
        mkdir -p "$FISH_CONF_DIR"
        if [ ! -f "$FISH_CONF" ] || ! grep -qF '.local/bin' "$FISH_CONF"; then
            cat > "$FISH_CONF" << 'FISHEOF'
# vidpolish shell setup (auto-generated by the vidpolish installer)
if not contains -- $HOME/.local/bin $PATH
    set -gx PATH $HOME/.local/bin $PATH
end
FISHEOF
            echo -e "${GREEN}OK${NC} created $FISH_CONF"
        fi
        ;;
    *)
        ;;
esac

export PATH="$HOME/.local/bin:$PATH"
echo -e "${GREEN}OK${NC} $("$INSTALL_PATH" version)"

# ---------------------------------------------------------------------------
# Step 5: fetch dependencies
# ---------------------------------------------------------------------------
step 5 "Fetching dependencies"

if [ "$VIDPOLISH_SKIP_DEPS" = "1" ]; then
    echo -e "${YELLOW}Skipped (VIDPOLISH_SKIP_DEPS=1)${NC}"
else
    echo "Checking ffmpeg and fetching deep-filter/auto-editor/resvg/font"
    echo "(each shows its own download progress; only missing ones are fetched):"
    echo ""
    if ! "$INSTALL_PATH" deps; then
        echo -e "${YELLOW}(warning) one or more dependencies could not be resolved; see ERROR lines above.${NC}"
        echo -e "${YELLOW}You can retry any time with: vidpolish deps${NC}"
    fi
fi

echo ""
echo -e "${GREEN}${BOLD}Installation complete!${NC}"
echo ""
echo -e "To start using vidpolish in your ${YELLOW}current${NC} terminal, run:"
echo ""
echo -e "  ${GREEN}source ~/.vidpolish/env${NC}"
echo ""
echo "New terminal sessions will pick it up automatically."
echo ""
echo "Get started:"
echo "  vidpolish process <input.mp4>   # polish a raw recording"
echo "  vidpolish ui                    # open the local Projects UI"
