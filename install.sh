#!/usr/bin/env bash
set -euo pipefail

# Yoink universal installer
# Installs the Yoink CLI from GitHub Releases (preferred) or builds from source.
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | sh
#
# Environment variables:
#   YOINK_INSTALL_DIR  — override install directory
#   YOINK_VERSION       — override version (default: latest)

REPO_OWNER="monojitgoswami69"
REPO_NAME="yoink"
GITHUB_BASE="https://github.com/${REPO_OWNER}/${REPO_NAME}"
LATEST_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"

err()  { echo "Error: $*" >&2; exit 1; }
info() { echo "$*"; }

# ── Detect OS ──────────────────────────────────────────────────────────────

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) err "Unsupported OS: $(uname -s). Use Linux, macOS, or Windows (Git Bash)." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) err "Unsupported architecture: $(uname -m). Use x86_64 or arm64." ;;
  esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

# Map to GoReleaser archive naming
case "$OS" in
  linux)   OS_NAME="Linux" ;;
  darwin)  OS_NAME="Darwin" ;;
  windows) OS_NAME="Windows" ;;
esac

case "$ARCH" in
  amd64) ARCH_NAME="x86_64" ;;
  arm64) ARCH_NAME="arm64" ;;
esac

# ── Determine version ──────────────────────────────────────────────────────

VERSION="${YOINK_VERSION:-}"
if [ -z "$VERSION" ]; then
  if command -v curl >/dev/null 2>&1; then
    VERSION="$(curl -sSfL "$LATEST_API" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')" || true
  fi
fi
if [ -z "$VERSION" ]; then
  # Fall back to building from source
  VERSION=""
fi

# ── Determine install location ────────────────────────────────────────────

if [ -n "${YOINK_INSTALL_DIR:-}" ]; then
  INSTALL_DIR="$YOINK_INSTALL_DIR"
elif [ "$(id -u)" -eq 0 ]; then
  INSTALL_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ] || [ -d "$HOME/bin" ]; then
  if [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
  else
    INSTALL_DIR="$HOME/bin"
  fi
else
  INSTALL_DIR="$HOME/.local/bin"
fi

# ── Download and install ──────────────────────────────────────────────────

if [ -n "$VERSION" ]; then
  # Preferred: download pre-built binary from GitHub Releases
  if [ "$OS" = "windows" ]; then
    ARCHIVE="${REPO_NAME}_${OS_NAME}_${ARCH_NAME}.zip"
    URL="${GITHUB_BASE}/releases/download/${VERSION}/${ARCHIVE}"
  else
    ARCHIVE="${REPO_NAME}_${OS_NAME}_${ARCH_NAME}.tar.gz"
    URL="${GITHUB_BASE}/releases/download/${VERSION}/${ARCHIVE}"
  fi

  info "→ Detecting platform: ${OS_NAME} ${ARCH_NAME}"
  info "→ Installing Yoink ${VERSION}"

  command -v curl >/dev/null 2>&1 || err "curl is required. Install it and try again."

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  info "→ Downloading ${ARCHIVE}..."
  curl -sSfL -o "${tmp}/${ARCHIVE}" "$URL" || err "Download failed. Check that version ${VERSION} exists."

  mkdir -p "$INSTALL_DIR"

  if [ "$OS" = "windows" ]; then
    command -v unzip >/dev/null 2>&1 || err "unzip is required on Windows."
    (cd "$tmp" && unzip -o -q "$ARCHIVE")
    install -m 0755 "${tmp}/${REPO_NAME}.exe" "${INSTALL_DIR}/${REPO_NAME}.exe" 2>/dev/null || \
      install -m 0755 "${tmp}/${REPO_NAME}" "${INSTALL_DIR}/${REPO_NAME}.exe" 2>/dev/null || \
      err "Could not find binary in archive."
    BINARY="${INSTALL_DIR}/${REPO_NAME}.exe"
  else
    command -v tar >/dev/null 2>&1 || err "tar is required."
    (cd "$tmp" && tar xzf "$ARCHIVE")
    install -m 0755 "${tmp}/${REPO_NAME}" "${INSTALL_DIR}/${REPO_NAME}" || err "Could not install binary."
    BINARY="${INSTALL_DIR}/${REPO_NAME}"
  fi

else
  # Fallback: build from source
  info "→ No pre-built binary found. Building from source..."

  command -v go >/dev/null 2>&1 || err "Go 1.25+ is required for building from source. https://go.dev/doc/install"
  command -v git >/dev/null 2>&1 || err "git is required for building from source."

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  info "→ Cloning repository..."
  git clone --depth=1 "https://github.com/${REPO_OWNER}/${REPO_NAME}.git" "${tmp}/${REPO_NAME}"

  info "→ Building..."
  (cd "${tmp}/${REPO_NAME}" && go build -ldflags "-s -w" -o "${REPO_NAME}" .)

  mkdir -p "$INSTALL_DIR"
  install -m 0755 "${tmp}/${REPO_NAME}/${REPO_NAME}" "${INSTALL_DIR}/${REPO_NAME}"
  BINARY="${INSTALL_DIR}/${REPO_NAME}"
fi

# ── Verify installation ────────────────────────────────────────────────────

info ""
info "✓ Yoink installed to: ${BINARY}"

if [ "$OS" != "windows" ]; then
  VER_OUTPUT="$("$BINARY" --version 2>&1)" || VER_OUTPUT="(version check failed)"
  info "  ${VER_OUTPUT}"
fi

# ── PATH management ───────────────────────────────────────────────────────

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    info ""
    info "Yoink is ready. Run: yoink help"
    ;;
  *)
    info ""
    info "⚠  ${INSTALL_DIR} is not in your PATH."

    # Detect shell
    SHELL_NAME="$(basename "${SHELL:-}")"

    case "$SHELL_NAME" in
      bash)
        RC_FILE="$HOME/.bashrc"
        info ""
        info "Add this line to ${RC_FILE}:"
        info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        info ""
        info "Then run: source ${RC_FILE}"
        ;;
      zsh)
        RC_FILE="$HOME/.zshrc"
        info ""
        info "Add this line to ${RC_FILE}:"
        info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        info ""
        info "Then run: source ${RC_FILE}"
        ;;
      fish)
        info ""
        info "Add this to ~/.config/fish/config.fish:"
        info "  fish_add_path ${INSTALL_DIR}"
        info ""
        info "Or run now: fish_add_path ${INSTALL_DIR}"
        ;;
      *)
        info ""
        info "Add ${INSTALL_DIR} to your PATH."
        info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        ;;
    esac

    info ""
    info "For this session:"
    info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    info ""
    info "Then run: yoink help"
    ;;
esac

info ""
info "Prerequisites at runtime:"
info "  • Docker Engine + Compose v2 (for build/heal/up)"
info "  • Git (for cloning)"
info ""
info "Quick start:"
info "  yoink setup                          # configure LLM provider"
info "  yoink init <github-url>              # clone, detect, generate, build"
info "  yoink up <project>                  # start the stack"
info "  yoink open <project>                # open in browser"
