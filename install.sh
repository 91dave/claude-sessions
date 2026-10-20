#!/bin/sh
set -eu

repo="91dave/claude-sessions"
install_dir="${CLAUDE_SESSIONS_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

asset="claude-sessions_${os}_${arch}.tar.gz"
url="https://github.com/$repo/releases/latest/download/$asset"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "Downloading $url"
curl -fsSL "$url" -o "$work/$asset"
tar -xzf "$work/$asset" -C "$work"
mkdir -p "$install_dir"
install -m 0755 "$work/claude-sessions" "$install_dir/claude-sessions"
echo "Installed claude-sessions to $install_dir"

case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) echo "Note: $install_dir is not on your PATH" ;;
esac
