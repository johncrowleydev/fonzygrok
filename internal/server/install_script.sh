#!/bin/sh
# Fonzygrok client installer
# Usage: curl -sSfL https://fonzygrok.com/install.sh | sh
#
# Installs the fonzygrok client binary to /usr/local/bin (or ~/.local/bin if
# no write access to /usr/local/bin).
#
# Environment variables:
#   FONZYGROK_VERSION  - version to install (default: latest)
#   FONZYGROK_INSTALL  - install directory (default: auto-detect)

set -e

REPO="johncrowleydev/fonzygrok"
BINARY="fonzygrok"

# --- Helpers ---

log()  { printf '  \033[1;34m→\033[0m %s\n' "$*"; }
ok()   { printf '  \033[1;32m✔\033[0m %s\n' "$*"; }
fail() { printf '  \033[1;31m✘\033[0m %s\n' "$*" >&2; exit 1; }

# --- Detect OS and arch ---

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       fail "Unsupported OS: $(uname -s). Only Linux and macOS are supported." ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)              fail "Unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
    esac
}

# --- Resolve version ---

resolve_version() {
    if [ -n "$FONZYGROK_VERSION" ]; then
        echo "$FONZYGROK_VERSION"
        return
    fi

    # Fetch latest release tag from GitHub API.
    if command -v curl >/dev/null 2>&1; then
        curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
    else
        fail "curl or wget is required"
    fi
}

# --- Resolve install directory ---

resolve_install_dir() {
    if [ -n "$FONZYGROK_INSTALL" ]; then
        echo "$FONZYGROK_INSTALL"
        return
    fi

    # Prefer /usr/local/bin if writable.
    if [ -w /usr/local/bin ]; then
        echo "/usr/local/bin"
    else
        # Fall back to ~/.local/bin.
        mkdir -p "$HOME/.local/bin"
        echo "$HOME/.local/bin"
    fi
}

# --- Download ---

download() {
    local url="$1" dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -sSfL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        fail "curl or wget is required"
    fi
}

# --- Main ---

main() {
    printf '\n  \033[1;36mFonzygrok Installer\033[0m\n\n'

    OS="$(detect_os)"
    ARCH="$(detect_arch)"
    log "Detected: ${OS}/${ARCH}"

    VERSION="$(resolve_version)"
    if [ -z "$VERSION" ]; then
        fail "Could not determine latest version. Set FONZYGROK_VERSION manually."
    fi
    log "Version: ${VERSION}"

    INSTALL_DIR="$(resolve_install_dir)"
    log "Install dir: ${INSTALL_DIR}"

    # Construct download URL.
    # Release asset naming: fonzygrok-<os>-<arch>
    ASSET="${BINARY}-${OS}-${ARCH}"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

    log "Downloading ${URL} ..."
    TMPFILE="$(mktemp)"
    download "$URL" "$TMPFILE" || fail "Download failed. Check that ${VERSION} has a release asset for ${OS}/${ARCH}."

    chmod +x "$TMPFILE"

    # Move to install dir. Use sudo if needed.
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
    else
        log "Requesting sudo to install to ${INSTALL_DIR} ..."
        sudo mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}"
    fi

    ok "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"

    # Check if install dir is in PATH.
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            printf '\n'
            printf '  \033[1;33m⚠\033[0m  %s is not in your PATH.\n' "$INSTALL_DIR"
            printf '     Add it with:\n'
            printf '       export PATH="%s:$PATH"\n' "$INSTALL_DIR"
            printf '     Or add that line to your ~/.bashrc / ~/.zshrc\n'
            ;;
    esac

    printf '\n'
    ok "Run: fonzygrok --port 3000"
    printf '\n'
}

main "$@"
