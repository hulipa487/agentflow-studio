#!/usr/bin/env bash
# Package AgentFlow Studio as a signed (.app) bundle, compressed tarball (for
# in-app auto-update), and drag-to-Applications .dmg on macOS.
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
STUDIO_ROOT="$(cd "$DESKTOP_DIR/.." && pwd)"

PLAIN_VER="${VERSION#v}"
case "$PLAIN_VER" in
  [0-9]*) ;;
  *) PLAIN_VER="0.1.0" ;;
esac
# Strip prerelease suffixes for CFBundleVersion numeric triplets.
BUNDLE_VER="${PLAIN_VER%%[-+]*}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

STAGE="$WORK/dmg-root"
APP="$STAGE/AgentFlow Studio.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

cp "$BINARY" "$APP/Contents/MacOS/agentflow-studio"
chmod 0755 "$APP/Contents/MacOS/agentflow-studio"

# Generate AppIcon.icns from the repo's crisp PNG logo/winres icons.
ICONSET="$WORK/AppIcon.iconset"
mkdir -p "$ICONSET"
SRC_LARGE="$STUDIO_ROOT/assets/brand/logo.png"
if [ ! -f "$SRC_LARGE" ]; then
  SRC_LARGE="$DESKTOP_DIR/winres/icon256.png"
fi

make_frame() {
  local px="$1"
  local out="$2"
  local exact="$DESKTOP_DIR/winres/icon${px}.png"
  if [ -f "$exact" ]; then
    cp "$exact" "$out"
  else
    sips -z "$px" "$px" "$SRC_LARGE" --out "$out" >/dev/null
  fi
}

make_frame 16 "$ICONSET/icon_16x16.png"
make_frame 32 "$ICONSET/icon_16x16@2x.png"
make_frame 32 "$ICONSET/icon_32x32.png"
make_frame 64 "$ICONSET/icon_32x32@2x.png"
make_frame 128 "$ICONSET/icon_128x128.png"
make_frame 256 "$ICONSET/icon_128x128@2x.png"
make_frame 256 "$ICONSET/icon_256x256.png"
make_frame 512 "$ICONSET/icon_256x256@2x.png"
make_frame 512 "$ICONSET/icon_512x512.png"
sips -z 1024 1024 "$SRC_LARGE" --out "$ICONSET/icon_512x512@2x.png" >/dev/null

iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"

cat <<EOF > "$APP/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>AgentFlow Studio</string>
  <key>CFBundleDisplayName</key>
  <string>AgentFlow Studio</string>
  <key>CFBundleIdentifier</key>
  <string>dev.agentflow.studio</string>
  <key>CFBundleExecutable</key>
  <string>agentflow-studio</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${PLAIN_VER}</string>
  <key>CFBundleVersion</key>
  <string>${BUNDLE_VER}</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

codesign --force --deep --sign - "$APP"

mkdir -p "$OUTDIR"

TAR_NAME="agentflow-studio-darwin-${ARCH}.tar.gz"
tar -C "$STAGE" -czf "$OUTDIR/$TAR_NAME" "AgentFlow Studio.app"

ln -s /Applications "$STAGE/Applications"
DMG_NAME="agentflow-studio-${VERSION}-darwin-${ARCH}.dmg"
rm -f "$OUTDIR/$DMG_NAME"
hdiutil create -volname "AgentFlow Studio" -srcfolder "$STAGE" -ov -format UDZO "$OUTDIR/$DMG_NAME" >/dev/null

echo "packaged $OUTDIR/$TAR_NAME and $OUTDIR/$DMG_NAME"
