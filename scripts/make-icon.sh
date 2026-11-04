#!/bin/sh
# Draws build/appicon.png and makes build/darwin/icons.icns from it, with Apple's own
# tools. Run it after changing the mark in make-icon.swift.
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
swift "$ROOT/scripts/make-icon.swift" "$ROOT/build/appicon.png"
SET=$(mktemp -d "${TMPDIR:-/tmp}/freeloop-icon.XXXXXX")/icon.iconset
mkdir -p "$SET"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$ROOT/build/appicon.png" --out "$SET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$ROOT/build/appicon.png" --out "$SET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$SET" -o "$ROOT/build/darwin/icons.icns"
rm -rf "$(dirname "$SET")"
echo "build/appicon.png and build/darwin/icons.icns"
