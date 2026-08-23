#!/usr/bin/env bash
set -euo pipefail

# Yoink universal installer with self-update
# Installs or upgrades the Yoink CLI from GitHub Releases.
# Falls back to building from source if no pre-built binary exists.
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/monojitgoswami69/yoink/main/install.sh | sh
#
# Flags:
#   --verbose   Show diagnostic info (OS, arch, versions, URLs)
#
# Environment variables:
#   YOINK_INSTALL_DIR  — override install directory
#   YOINK_VERSION       — override version (default: latest from GitHub)

REPO_OWNER="monojitgoswami69"
REPO_NAME="yoink"
GITHUB_BASE="https://github.com/${REPO_OWNER}/${REPO_NAME}"
LATEST_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"

VERBOSE=false
for arg in "$@"; do
  case "$arg" in
    --verbose|-v) VERBOSE=true ;;
  esac
done

err()  { echo "Error: $*" >&2; exit 1; }
info() { echo "$*"; }
vinfo() { $VERBOSE && echo "[verbose] $*" || true; }

# ── Detect OS and architecture ─────────────────────────────────────────────

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

case "$OS" in
  linux)   OS_NAME="Linux" ;;
  darwin)  OS_NAME="Darwin" ;;
  windows) OS_NAME="Windows" ;;
esac

case "$ARCH" in
  amd64) ARCH_NAME="x86_64" ;;
  arm64) ARCH_NAME="arm64" ;;
esac

vinfo "OS: ${OS_NAME}"
vinfo "Architecture: ${ARCH_NAME}"

# ── Find existing installation ─────────────────────────────────────────────

find_existing_binary() {
  # Check PATH first
  if command -v yoink >/dev/null 2>&1; then
    command -v yoink
    return 0
  fi
  # Check standard locations
  for dir in /usr/local/bin "$HOME/.local/bin" "$HOME/bin"; do
    if [ -x "${dir}/yoink" ]; then
      echo "${dir}/yoink"
      return 0
    fi
  done
  return 1
}

get_installed_version() {
  local bin="$1"
  local ver
  ver="$("$bin" --version 2>/dev/null | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+' | head -1)" || true
  # Normalize: ensure leading v
  case "$ver" in
    v*) ;;
    *) ver="v${ver}" ;;
  esac
  echo "$ver"
}

# ── Fetch latest version from GitHub ───────────────────────────────────────

get_latest_version() {
  command -v curl >/dev/null 2>&1 || return 1
  curl -sSfL "$LATEST_API" 2>/dev/null | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/' || true
}

# ── Compare semantic versions ──────────────────────────────────────────────

version_ge() {
  # Returns 0 if $1 >= $2, 1 otherwise
  local a="$1" b="$2"
  a="${a#v}" ; b="${b#v}"
  local a_major a_minor a_patch b_major b_minor b_patch
  IFS='.' read -r a_major a_minor a_patch <<< "$a"
  IFS='.' read -r b_major b_minor b_patch <<< "$b"
  if [ "$a_major" -gt "$b_major" ]; then return 0; fi
  if [ "$a_major" -lt "$b_major" ]; then return 1; fi
  if [ "$a_minor" -gt "$b_minor" ]; then return 0; fi
  if [ "$a_minor" -lt "$b_minor" ]; then return 1; fi
  if [ "$a_patch" -ge "$b_patch" ]; then return 0; fi
  return 1
}

# ── Determine install location ─────────────────────────────────────────────

determine_install_dir() {
  if [ -n "${YOINK_INSTALL_DIR:-}" ]; then
    echo "$YOINK_INSTALL_DIR"
    return
  fi
  # If an existing binary is found, use its directory
  if [ -n "${EXISTING_BIN:-}" ] && [ -d "$(dirname "$EXISTING_BIN")" ]; then
    dirname "$EXISTING_BIN"
    return
  fi
  if [ "$(id -u)" -eq 0 ]; then
    echo "/usr/local/bin"
  elif [ -d "$HOME/.local/bin" ]; then
    echo "$HOME/.local/bin"
  elif [ -d "$HOME/bin" ]; then
    echo "$HOME/bin"
  else
    echo "$HOME/.local/bin"
  fi
}

# ── Check for existing installation ────────────────────────────────────────

EXISTING_BIN=""
INSTALLED_VER=""

if EXISTING_BIN="$(find_existing_binary 2>/dev/null)"; then
  INSTALLED_VER="$(get_installed_version "$EXISTING_BIN")"
  vinfo "Existing binary: ${EXISTING_BIN}"
  vinfo "Installed version: ${INSTALLED_VER:-unknown}"
else
  vinfo "No existing yoink installation found"
fi

# ── Determine target version ───────────────────────────────────────────────

TARGET_VER="${YOINK_VERSION:-}"
if [ -z "$TARGET_VER" ]; then
  TARGET_VER="$(get_latest_version)" || true
fi

if [ -z "$TARGET_VER" ]; then
  # No GitHub release available — fall through to source build
  vinfo "No GitHub release found, will build from source"
else
  vinfo "Latest GitHub release: ${TARGET_VER}"
fi

# ── Version comparison and decision ────────────────────────────────────────

if [ -n "$INSTALLED_VER" ] && [ -n "$TARGET_VER" ]; then
  if [ "$INSTALLED_VER" = "$TARGET_VER" ]; then
    info "Yoink already installed."
    info "Current version: ${INSTALLED_VER}"
    info ""
    info "Run: yoink help"
    exit 0
  elif version_ge "$INSTALLED_VER" "$TARGET_VER"; then
    info "Downgrading Yoink:"
    info "  ${INSTALLED_VER} -> ${TARGET_VER}"
    info ""
  else
    info "Upgrading Yoink:"
    info "  ${INSTALLED_VER} -> ${TARGET_VER}"
    info ""
  fi
fi

# ── Determine install location ────────────────────────────────────────────

INSTALL_DIR="$(determine_install_dir)"
vinfo "Install directory: ${INSTALL_DIR}"

# Check if we need root
NEEDS_ROOT=false
if [ -n "$EXISTING_BIN" ]; then
  if [ ! -w "$(dirname "$EXISTING_BIN")" ]; then
    NEEDS_ROOT=true
  fi
elif [ "$INSTALL_DIR" = "/usr/local/bin" ] && [ "$(id -u)" -ne 0 ]; then
  NEEDS_ROOT=true
fi

if $NEEDS_ROOT && [ "$(id -u)" -ne 0 ]; then
  info "Install location requires root access: ${INSTALL_DIR}"
  info "Re-run with sudo:"
  info "  curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/install.sh | sudo sh"
  exit 1
fi

# ── Download and install ──────────────────────────────────────────────────

command -v curl >/dev/null 2>&1 || err "curl is required. Install it and try again."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -n "$TARGET_VER" ]; then
  if [ "$OS" = "windows" ]; then
    ARCHIVE="${REPO_NAME}_${OS_NAME}_${ARCH_NAME}.zip"
  else
    ARCHIVE="${REPO_NAME}_${OS_NAME}_${ARCH_NAME}.tar.gz"
  fi
  URL="${GITHUB_BASE}/releases/download/${TARGET_VER}/${ARCHIVE}"
  vinfo "Download URL: ${URL}"

  info "Downloading ${ARCHIVE}..."
  curl -sSfL -o "${tmp}/${ARCHIVE}" "$URL" || err "Download failed. Check that version ${TARGET_VER} exists."

  mkdir -p "$INSTALL_DIR"

  if [ "$OS" = "windows" ]; then
    command -v unzip >/dev/null 2>&1 || err "unzip is required on Windows."
    (cd "$tmp" && unzip -o -q "$ARCHIVE")
    NEW_BINARY="${tmp}/${REPO_NAME}.exe"
    [ -f "$NEW_BINARY" ] || NEW_BINARY="${tmp}/${REPO_NAME}"
    [ -f "$NEW_BINARY" ] || err "Could not find binary in archive."
    BINARY="${INSTALL_DIR}/${REPO_NAME}.exe"
  else
    command -v tar >/dev/null 2>&1 || err "tar is required."
    (cd "$tmp" && tar xzf "$ARCHIVE")
    NEW_BINARY="${tmp}/${REPO_NAME}"
    [ -f "$NEW_BINARY" ] || err "Could not find binary in archive."
    BINARY="${INSTALL_DIR}/${REPO_NAME}"
  fi

  # Preserve existing binary until replacement succeeds
  if [ -f "$BINARY" ]; then
    cp "$BINARY" "${BINARY}.bak"
    vinfo "Backed up existing binary to ${BINARY}.bak"
  fi

  install -m 0755 "$NEW_BINARY" "$BINARY" || err "Could not install binary."

  # Clean up backup on success
  rm -f "${BINARY}.bak"

else
  info "No pre-built binary found. Building from source..."
  command -v go >/dev/null 2>&1 || err "Go 1.25+ is required for building from source. https://go.dev/doc/install"
  command -v git >/dev/null 2>&1 || err "git is required for building from source."

  info "Cloning repository..."
  git clone --depth=1 "https://github.com/${REPO_OWNER}/${REPO_NAME}.git" "${tmp}/${REPO_NAME}"

  info "Building..."
  (cd "${tmp}/${REPO_NAME}" && go build -ldflags "-s -w" -o "${REPO_NAME}" .)

  mkdir -p "$INSTALL_DIR"
  BINARY="${INSTALL_DIR}/${REPO_NAME}"

  if [ -f "$BINARY" ]; then
    cp "$BINARY" "${BINARY}.bak"
  fi

  install -m 0755 "${tmp}/${REPO_NAME}/${REPO_NAME}" "$BINARY" || err "Could not install binary."
  rm -f "${BINARY}.bak"
fi

# ── Verify installation ────────────────────────────────────────────────────

info ""
info "Yoink installed to: ${BINARY}"

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
    info "${INSTALL_DIR} is not in your PATH."

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
info "  Docker Engine + Compose v2 (for build/heal/up)"
info "  Git (for cloning)"
info ""
info "Quick start:"
info "  yoink setup"
info "  yoink init <github-url>"
info "  yoink up <project>"
info "  yoink open <project>"
