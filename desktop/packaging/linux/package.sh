#!/usr/bin/env bash
# Package AgentFlow Studio for Linux as both a .deb package and a portable
# .tar.gz archive (used for user-local installation and in-app auto-update).
#
# Usage: package.sh <version> <arch> <binary> <outdir>
set -euo pipefail

if [ "$#" -lt 4 ]; then
  echo "usage: $0 <version> <arch> <binary> <outdir>" >&2
  exit 2
fi

VERSION="$1"
ARCH="$2"
BINARY="$3"
OUTDIR="$4"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DESKTOP_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

DEB_VER="${VERSION#v}"
case "$DEB_VER" in
  [0-9]*) ;;
  *) DEB_VER="0.1.0~${DEB_VER}" ;;
esac
# dpkg-deb forbids underscores in version strings.
DEB_VER="${DEB_VER//_/-}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# 1. Build Debian package (.deb).
PKG_ROOT="$WORK/deb"
mkdir -p \
  "$PKG_ROOT/DEBIAN" \
  "$PKG_ROOT/usr/bin" \
  "$PKG_ROOT/usr/share/applications"

install -m 0755 "$BINARY" "$PKG_ROOT/usr/bin/agentflow-studio"
install -m 0644 "$SCRIPT_DIR/agentflow-studio.desktop" "$PKG_ROOT/usr/share/applications/agentflow-studio.desktop"

for sz in 16 24 32 48 64 128 256; do
  SRC="$DESKTOP_DIR/winres/icon${sz}.png"
  if [ -f "$SRC" ]; then
    DST_DIR="$PKG_ROOT/usr/share/icons/hicolor/${sz}x${sz}/apps"
    mkdir -p "$DST_DIR"
    install -m 0644 "$SRC" "$DST_DIR/agentflow-studio.png"
  fi
done

cat <<EOF > "$PKG_ROOT/DEBIAN/control"
Package: agentflow-studio
Version: ${DEB_VER}
Section: devel
Priority: optional
Architecture: ${ARCH}
Maintainer: AgentFlow <152088546+hulipa487@users.noreply.github.com>
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Description: Visual workbench for AgentFlow LLM agents
 Build, apply live, chat and trace every turn of an AgentFlow runtime.
EOF

cat <<'EOF' > "$PKG_ROOT/DEBIAN/postinst"
#!/bin/sh
set -e
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q -t /usr/share/icons/hicolor || true
fi
EOF
chmod 0755 "$PKG_ROOT/DEBIAN/postinst"

mkdir -p "$OUTDIR"
DEB_NAME="agentflow-studio_${DEB_VER}_${ARCH}.deb"
dpkg-deb --root-owner-group --build "$PKG_ROOT" "$OUTDIR/$DEB_NAME" >/dev/null

# 2. Build portable tarball (.tar.gz) for ~/.local/ installation and auto-update.
TAR_STAGE="$WORK/tar"
mkdir -p "$TAR_STAGE"
install -m 0755 "$BINARY" "$TAR_STAGE/agentflow-studio"
install -m 0644 "$SCRIPT_DIR/agentflow-studio.desktop" "$TAR_STAGE/agentflow-studio.desktop"
install -m 0644 "$DESKTOP_DIR/winres/icon256.png" "$TAR_STAGE/icon256.png"
install -m 0755 "$SCRIPT_DIR/install.sh" "$TAR_STAGE/install.sh"

TAR_NAME="agentflow-studio-linux-${ARCH}.tar.gz"
tar -C "$TAR_STAGE" --owner=0 --group=0 -czf "$OUTDIR/$TAR_NAME" .

echo "packaged $OUTDIR/$DEB_NAME and $OUTDIR/$TAR_NAME"
