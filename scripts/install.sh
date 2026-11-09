#!/bin/sh
# NFR-REL-006: leave exactly one Freeloop.app on this Mac, built from this checkout's
# current commit, so the one that opens is the current one. Run from a terminal, not
# from inside Freeloop, which it quits.
#
#   make install                              build bin/Freeloop.app and keep it there
#   make install INSTALL_DIR=~/Applications   then move it to ~/Applications/Freeloop.app
#   make install OPEN=1                       open it afterwards
#   SHIPPED=1 make install                    the build users get, which reads no development switch
#
# In order: quit Freeloop (never force it: it may be asking to confirm while agents
# work), put the old bundle in the Trash, build, check the new bundle is this commit's
# and signed, place it, put every other Freeloop bundle in the Trash (found by its
# bundle identifier through Spotlight, and in the two Applications folders), and tell
# Launch Services about the new one. A failed build puts the old bundle back. Nothing
# is deleted: the Trash holds what moved. A disk image is left alone and named.
#
# For tests only: INSTALL_BUILD (the command that builds; default `make package`),
# LSREGISTER and OSASCRIPT (the programs that update Launch Services and ask the app
# to quit; `true` skips them), INSTALL_SPOTLIGHT=0 (do not ask Spotlight for other
# copies) and INSTALL_QUIT_TRIES (half seconds to wait for the app to quit; 60).
set -eu

[ "$(uname)" = Darwin ] || { echo "make install is for a Mac: it replaces a macOS app. On Windows and Linux, build with make windows or make linux-package." >&2; exit 2; }

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
PLIST=build/darwin/Info.plist
plist() { plutil -extract "$1" raw -o - "${2:-$PLIST}" 2>/dev/null || true; }
APP_NAME=$(plist CFBundleName)
EXEC=$(plist CFBundleExecutable)
BUNDLE_ID=$(plist CFBundleIdentifier)
[ -n "$APP_NAME" ] && [ -n "$EXEC" ] && [ -n "$BUNDLE_ID" ] || { echo "install: $PLIST does not name the app" >&2; exit 2; }

BUILT="$ROOT/bin/$APP_NAME.app"
TRASH="$HOME/.Trash"
LSREGISTER=${LSREGISTER:-/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister}
OSASCRIPT=${OSASCRIPT:-osascript}
QUIT_TRIES=${INSTALL_QUIT_TRIES:-60}
INSTALL_BUILD=${INSTALL_BUILD:-make package}
INSTALL_SPOTLIGHT=${INSTALL_SPOTLIGHT:-1}
INSTALL_DIR=${INSTALL_DIR:-}
case "$INSTALL_DIR" in "~"*) INSTALL_DIR="$HOME${INSTALL_DIR#"~"}" ;; esac
DEST="$BUILT"
[ -z "$INSTALL_DIR" ] || DEST="$INSTALL_DIR/$APP_NAME.app"
RUNNING="/$APP_NAME.app/Contents/MacOS/$EXEC"

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

# The commit the checkout is at, written as the build tasks write it (Taskfile.yml).
stamp() {
	c=$(git -c safe.directory='*' rev-parse --short=7 HEAD 2>/dev/null) || return 0
	if [ -n "$(git -c safe.directory='*' status --porcelain 2>/dev/null)" ]; then c="$c+dirty"; fi
	printf '%s' "$c"
}

# --- Not from inside the app: quitting it would end this command ---------------------
p=$$
while [ -n "$p" ] && [ "$p" -gt 1 ]; do
	case "$(ps -o command= -p "$p" 2>/dev/null)" in
	*"$RUNNING"*) die "this is running inside $APP_NAME, which it has to quit. Run it from a terminal." ;;
	esac
	p=$(ps -o ppid= -p "$p" 2>/dev/null | tr -d ' ')
done

# --- Quit the running app --------------------------------------------------------------
if pgrep -f "$RUNNING" >/dev/null 2>&1; then
	say "Quitting ${APP_NAME}…"
	"$OSASCRIPT" -e "tell application id \"$BUNDLE_ID\" to quit" >/dev/null 2>&1 || true
	i=0
	while pgrep -f "$RUNNING" >/dev/null 2>&1; do
		i=$((i + 1))
		[ $i -le "$QUIT_TRIES" ] || die "$APP_NAME is still running. It may be asking whether to quit while agents work: answer it, or quit it yourself (⌘Q), then run this again."
		sleep 0.5
	done
fi

# --- Move something to the Trash, where it can be got back -----------------------------
trashed=""
trash() {
	[ -e "$1" ] || return 0
	n=0
	dest="$TRASH/$APP_NAME-old-$(date +%H%M%S)"
	while [ -e "$dest.app" ]; do
		n=$((n + 1))
		dest="$TRASH/$APP_NAME-old-$(date +%H%M%S)-$n"
	done
	mkdir -p "$TRASH"
	mv "$1" "$dest.app" || die "could not move $1 to the Trash (is it yours to move? try moving it by hand)"
	"$LSREGISTER" -u "$dest.app" >/dev/null 2>&1 || true
	trashed="$dest.app"
	say "  moved $1 to the Trash"
}

# --- The old bundle out of the way, then build -----------------------------------------
old=""
if [ -e "$BUILT" ]; then
	trash "$BUILT"
	old="$trashed"
fi
# What went wrong leaves the old bundle where it was, and what was built in its place
# in the Trash.
rollback() {
	[ -e "$BUILT" ] && trash "$BUILT"
	if [ -n "$old" ]; then
		mkdir -p "$(dirname "$BUILT")"
		mv "$old" "$BUILT" && say "The old $APP_NAME is back where it was."
	fi
	die "$1"
}

say "Building ${APP_NAME}…"
sh -c "$INSTALL_BUILD" || rollback "the build failed."

# --- Check what was built --------------------------------------------------------------
[ -d "$BUILT" ] || rollback "the build left no $BUILT."
codesign --verify --deep --strict "$BUILT" 2>/dev/null || rollback "the new bundle is not validly signed."
want=$(stamp)
have=$(plist FreeloopCommit "$BUILT/Contents/Info.plist")
if [ -n "$want" ] && [ "$have" != "$want" ]; then
	rollback "the new bundle says it is from ${have:-no commit}, and this checkout is at $want. Something built another tree."
fi

# --- Place it, and clear every other copy ----------------------------------------------
if [ "$DEST" != "$BUILT" ]; then
	mkdir -p "$INSTALL_DIR"
	trash "$DEST"
	mv "$BUILT" "$DEST" || die "could not move the new bundle to $DEST"
fi

found=""
if [ "$INSTALL_SPOTLIGHT" != 0 ]; then
	found=$(mdfind "kMDItemCFBundleIdentifier == '$BUNDLE_ID'" 2>/dev/null || true)
fi
# One path a line, since a path may hold a space.
list=$(mktemp "${TMPDIR:-/tmp}/freeloop-install.XXXXXX")
trap 'rm -f "$list"' EXIT
printf '%s\n' "$found" "/Applications/$APP_NAME.app" "$HOME/Applications/$APP_NAME.app" "$ROOT/bin/$APP_NAME.app" > "$list"
while IFS= read -r path; do
	[ -n "$path" ] && [ -d "$path" ] || continue
	[ "$path" = "$DEST" ] && continue
	case "$path" in "$TRASH"/*) continue ;; esac
	trash "$path"
done < "$list"
"$LSREGISTER" -f "$DEST" >/dev/null 2>&1 || true

say ""
say "$APP_NAME ${have:+($have) }is at $DEST"
for dmg in "$HOME"/Downloads/"$APP_NAME"*.dmg; do
	[ -e "$dmg" ] && say "A disk image is still at $dmg; it holds an older build, so do not open the app from it."
done
[ "${OPEN:-}" != 1 ] || open "$DEST"
