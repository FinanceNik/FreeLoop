#!/bin/sh
# Draws build/appicon.png and makes the icons every platform needs from it:
# build/darwin/icons.icns with Apple's own tools, build/windows/icon.ico with
# the pinned wails3 (`make tools`), and the PNGs under build/linux/icons. Run it
# after changing the mark in make-icon.swift; ICONS_ONLY=1 skips the drawing and
# only remakes the icons from the committed build/appicon.png.
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WAILS="$(go env GOPATH)/bin/wails3"
[ "${ICONS_ONLY:-}" = 1 ] || swift "$ROOT/scripts/make-icon.swift" "$ROOT/build/appicon.png"
SET=$(mktemp -d "${TMPDIR:-/tmp}/freeloop-icon.XXXXXX")/icon.iconset
mkdir -p "$SET"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$ROOT/build/appicon.png" --out "$SET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$ROOT/build/appicon.png" --out "$SET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$SET" -o "$ROOT/build/darwin/icons.icns"
rm -rf "$(dirname "$SET")"
# Windows: one .ico with every size the shell asks for (NFR-XPLAT-003).
"$WAILS" generate icons -input "$ROOT/build/appicon.png" -sizes 256,128,64,48,32,16 \
  -windowsfilename "$ROOT/build/windows/icon.ico" -macfilename ""
# Linux: the hicolor theme sizes the .deb and the AppImage install.
mkdir -p "$ROOT/build/linux/icons"
for size in 48 128 256 512; do
  sips -z "$size" "$size" "$ROOT/build/appicon.png" --out "$ROOT/build/linux/icons/${size}x${size}.png" >/dev/null
done
echo "build/appicon.png, build/darwin/icons.icns, build/windows/icon.ico and build/linux/icons"
