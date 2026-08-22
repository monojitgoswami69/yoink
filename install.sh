#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${YOINK_INSTALL_DIR:-$HOME/.local/bin}"
REPO_URL="https://github.com/monojitgoswami69/yoink.git"

err() { echo "error: $*" >&2; exit 1; }

command -v go  >/dev/null || err "Go 1.21+ required. https://go.dev/doc/install"
command -v git >/dev/null || err "git required."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "→ cloning yoink into $tmp..."
git -C "$tmp" clone --depth=1 "$REPO_URL"

echo "→ building..."
( cd "$tmp/yoink" && go build -ldflags "-s -w" -o yoink . )

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/yoink/yoink" "$INSTALL_DIR/yoink"

echo
echo "✓ installed → $INSTALL_DIR/yoink"
case ":${PATH}:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "  add '$INSTALL_DIR' to your PATH:"
     echo "    export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
echo
echo "next steps:"
echo "  yoink setup                                  # configure provider + API key"
echo "  yoink init https://github.com/<owner>/<repo> # clone, generate, build/heal"
echo "  yoink up <repo>                              # start the generated stack"
echo "  yoink dash <repo>                            # open the dashboard"
echo
echo "  (Docker Engine + Compose v2 is required at run time, not at install time.)"
