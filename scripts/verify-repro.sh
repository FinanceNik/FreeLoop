#!/bin/sh
# NFR-SEC-004: builds the app twice from one commit, in two fresh clones at
# different paths with separate Go build caches, and compares every file of the
# bundle and the bill of materials byte for byte. Exits 1 if any differ.
#
#   scripts/verify-repro.sh [commit or tag]      (default HEAD)
#
# Same result needs the same Go (go.mod's toolchain), Node and Xcode tools on
# both sides: their linker and SDK shape the binary. A DMG is not compared: hdiutil
# stamps it with the time. A release signed with a Developer ID has a dated
# signature; to compare one, remove it first (codesign --remove-signature) and
# sign the same way (ad hoc) here.
#
# Per platform (NFR-XPLAT-003): on a Mac it compares the app bundle and its bill
# (`make release RELEASE_PLATFORMS=mac`), and on every host the two Windows exes
# (`make windows-exe`: pure Go, cross-compiled, so any host makes the same bytes).
# The Windows installer and zip are not compared: makensis and zip stamp times.
# Linux builds in Docker (`make linux-package`) and is not compared here; its
# .deb and AppImage carry times too.
set -eu
ref="${1:-HEAD}"
repo="$(git rev-parse --show-toplevel)"
commit="$(git -C "$repo" rev-parse --verify "$ref^{commit}")"
work="$(mktemp -d "${TMPDIR:-/tmp}/freeloop-repro.XXXXXX")"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

build() { # folder under $work
	git clone -q --no-hardlinks "$repo" "$work/$1"
	git -C "$work/$1" checkout -q "$commit"
	(
		cd "$work/$1"
		if [ "$mac" = 1 ]; then
			GOCACHE="$work/cache-$1" GOFLAGS=-modcacherw make release RELEASE_PLATFORMS=mac >"$work/$1.log" 2>&1
		fi
		GOCACHE="$work/cache-$1" GOFLAGS=-modcacherw make windows-exe >>"$work/$1.log" 2>&1
	) || {
		tail -20 "$work/$1.log"
		echo "the build in $1 failed" >&2
		exit 1
	}
	files="windows-amd64/Freeloop.exe windows-arm64/Freeloop.exe"
	[ "$mac" = 1 ] && files="Freeloop.app release/Freeloop.sbom.cdx.json $files"
	# shellcheck disable=SC2086 # the list is split on purpose
	(cd "$work/$1/bin" && find $files -type f | LC_ALL=C sort | xargs $sha) >"$work/$1.sums"
}
mac=0
[ "$(uname -s)" = Darwin ] && mac=1
sha="shasum -a 256"
command -v shasum >/dev/null 2>&1 || sha=sha256sum
echo "building $commit twice; this takes some minutes"
build first
build a-second-folder-with-a-longer-name
if diff "$work/first.sums" "$work/a-second-folder-with-a-longer-name.sums"; then
	echo "reproducible: $(wc -l <"$work/first.sums" | tr -d ' ') files match"
	if [ -f "$work/first/bin/release/BUILDINFO.txt" ]; then cat "$work/first/bin/release/BUILDINFO.txt"; fi
else
	echo "NOT reproducible: the files above differ" >&2
	exit 1
fi
