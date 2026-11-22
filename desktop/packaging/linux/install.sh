#!/usr/bin/env bash
# User-local installer for the Linux portable tarball: installs under ~/.local/
# so AgentFlow Studio can self-update cleanly without root privileges.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}"
APP_DIR="$DATA_DIR/applications"
ICON_DIR="$DATA_DIR/icons/hicolor/256x256/apps"

mkdir -p "$BIN_DIR" "$APP_DIR" "$ICON_DIR"

install -m 0755 "$DIR/agentflow-studio" "$BIN_DIR/agentflow-studio"
install -m 0644 "$DIR/icon256.png" "$ICON_DIR/agentflow-studio.png"
sed "s|^Exec=agentflow-studio$|Exec=$BIN_DIR/agentflow-studio|" \
  "$DIR/agentflow-studio.desktop" > "$APP_DIR/agentflow-studio.desktop"

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$APP_DIR" >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -f -t "$DATA_DIR/icons/hicolor" >/dev/null 2>&1 || true
fi

echo "Installed AgentFlow Studio to $BIN_DIR/agentflow-studio"
