#!/bin/sh
# GUI-050: builds whisper.cpp's whisper-cli for the Mac into bin/whisper-cli, which
# `make package` puts in the app beside Freeloop. whisper.cpp publishes builds for
# Windows and Linux, which the app downloads when asked, but none for the Mac, so
# the Mac build makes its own from the pinned source: Apple silicon, macOS 14, Metal
# with its shaders inside the program, no library outside the system's, and the
# same bytes from any folder (NFR-SEC-004). Needs cmake: with Homebrew, a terminal and the
# user's yes it installs it first (see ensure_cmake); FREELOOP_INSTALL_CMAKE=1 says yes
# beforehand, for a machine no one sits at, and 0 says no.
#
#   scripts/whisper-cli.sh            build bin/whisper-cli
#   scripts/whisper-cli.sh --need     only make sure it can be built (or is built), so
#                                     `make install` asks before it touches the old app
set -eu
VERSION=1.9.5
SHA256=ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720
URL="https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v$VERSION.tar.gz"

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/bin/whisper-cli"
if [ -x "$OUT" ] && [ "$(cat "$OUT.version" 2>/dev/null)" = "$VERSION" ]; then
	exit 0
fi

# cmake builds whisper.cpp. Installing software on the Mac is the user's to allow, so
# this asks first and, with no terminal to ask at, installs only when told to.
ensure_cmake() {
	command -v cmake >/dev/null 2>&1 && return 0
	if command -v brew >/dev/null 2>&1; then
		answer=n
		case "${FREELOOP_INSTALL_CMAKE:-}" in
		1) answer=y ;;
		0) ;;
		*)
			if [ -t 0 ] && [ -t 2 ]; then
				printf "Voice input's speech program is built with cmake, which this Mac does not have.\nInstall it with Homebrew now (brew install cmake)? [Y/n] " >&2
				read -r answer || answer=n
				[ -n "$answer" ] || answer=y
			fi
			;;
		esac
		case "$answer" in
		y | Y | yes | YES | Yes)
			brew install cmake >&2 || { echo "brew could not install cmake" >&2; exit 1; }
			command -v cmake >/dev/null 2>&1 && return 0
			echo "cmake was installed but is not on the PATH; open a new terminal and run this again" >&2
			exit 1
			;;
		esac
		echo "building whisper-cli needs cmake: brew install cmake (then run this again), or set FREELOOP_INSTALL_CMAKE=1 to have it installed" >&2
	else
		echo "building whisper-cli needs cmake, and Homebrew (https://brew.sh) is not here to install it: install cmake some other way, or copy a built bin/whisper-cli, bin/whisper-cli.version, bin/whisper-cli.json and bin/whisper-cli.LICENSE from another Mac" >&2
	fi
	exit 1
}
ensure_cmake
[ "${1:-}" != --need ] || exit 0

WORK="$ROOT/bin/whisper-build"
TARBALL="$WORK/whisper.cpp-$VERSION.tar.gz"
mkdir -p "$WORK"
if [ ! -f "$TARBALL" ]; then
	curl -fsSL -o "$TARBALL.part" "$URL"
	mv "$TARBALL.part" "$TARBALL"
fi
# GitHub makes the tarball on request; if its bytes ever change, this says so rather
# than building something else.
got=$(shasum -a 256 "$TARBALL" | cut -d' ' -f1)
if [ "$got" != "$SHA256" ]; then
	rm -f "$TARBALL"
	echo "whisper.cpp $VERSION's source is not the pinned one ($got); not building it" >&2
	exit 1
fi
rm -rf "$WORK/src" "$WORK/out"
mkdir -p "$WORK/src"
tar -xzf "$TARBALL" -C "$WORK/src"
SRC="$WORK/src/whisper.cpp-$VERSION"
# The source's and the build's folders are left out of what is compiled in, so two
# checkouts at different paths make the same program.
MAP="-ffile-prefix-map=$SRC=whisper.cpp -ffile-prefix-map=$WORK/out=build"
# ggml writes the commit of the git repository around its source into the program:
# here Freeloop's, which would change it with every commit. Git is kept from looking
# above the source.
export ZERO_AR_DATE=1 MACOSX_DEPLOYMENT_TARGET=14.0 GIT_CEILING_DIRECTORIES="$WORK/src"
cmake -S "$SRC" -B "$WORK/out" -DCMAKE_BUILD_TYPE=Release \
	-DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF \
	-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DGGML_METAL_MACOSX_VERSION_MIN=14.0 \
	-DCMAKE_OSX_ARCHITECTURES=arm64 -DCMAKE_OSX_DEPLOYMENT_TARGET=14.0 \
	-DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF -DWHISPER_SDL2=OFF -DWHISPER_CURL=OFF \
	"-DCMAKE_C_FLAGS=$MAP" "-DCMAKE_CXX_FLAGS=$MAP" "-DCMAKE_OBJC_FLAGS=$MAP" >"$WORK/cmake.log" 2>&1 ||
	{ tail -20 "$WORK/cmake.log" >&2; exit 1; }
cmake --build "$WORK/out" --config Release --target whisper-cli -j "$(sysctl -n hw.ncpu)" >"$WORK/build.log" 2>&1 ||
	{ tail -20 "$WORK/build.log" >&2; exit 1; }
BUILT="$WORK/out/bin/whisper-cli"

# What the app may carry: one architecture, the minimum system, the system's libraries.
test "$(lipo -archs "$BUILT")" = arm64 || { echo "whisper-cli is not arm64 only" >&2; exit 1; }
minos=$(otool -l "$BUILT" | awk '/LC_BUILD_VERSION/{f=1} f&&/minos/{print $2; exit}')
test "$minos" = 14.0 || { echo "whisper-cli needs macOS $minos, not 14.0" >&2; exit 1; }
if otool -L "$BUILT" | tail -n +2 | grep -vE '^[[:space:]]+(/usr/lib/|/System/Library/)'; then
	echo "whisper-cli links the libraries above, which a Mac does not have" >&2
	exit 1
fi
cp "$BUILT" "$OUT.tmp"
mv "$OUT.tmp" "$OUT"
# What the bill of materials and the notices say about it (internal/sbom, Native).
cp "$SRC/LICENSE" "$OUT.LICENSE"
miniaudio=$(sed -n 's/^miniaudio - v\([0-9.]*\) .*/\1/p' "$SRC/examples/miniaudio.h" | head -1)
cat >"$OUT.json" <<JSON
{
  "name": "whisper.cpp",
  "version": "$VERSION",
  "purl": "pkg:github/ggml-org/whisper.cpp@v$VERSION",
  "sha256": "$SHA256",
  "license": "MIT",
  "includes": "ggml (MIT, the same authors), miniaudio $miniaudio (public domain or MIT-0) and stb_vorbis (public domain or MIT)",
  "licenseFile": "whisper-cli.LICENSE"
}
JSON
echo "$VERSION" >"$OUT.version"
echo "bin/whisper-cli (whisper.cpp $VERSION)"
