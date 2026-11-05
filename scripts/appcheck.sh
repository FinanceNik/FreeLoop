#!/bin/sh
# Drives the real interface on the real engine in a real WebKit view, on scratch data,
# with the demo provider: what the unit tests cannot see. Bugs it was written for lived
# only in WebKit or between the page and the engine: confirm() answering no, the view
# no longer following an agent, a setting that never saved. The scenario is in
# appcheck/scenario.js. Needs no key and leaves the login keychain alone.
#
# It runs the served build, since a packaged app cannot be scripted, so a native dialog
# is the one thing not covered: the scenario answers that call itself.
#
#   scripts/appcheck.sh
set -eu

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(dirname "$HERE")
PORT=${APPCHECK_PORT:-34391}
DATA=$(mktemp -d "${TMPDIR:-/tmp}/freeloop-appcheck.XXXXXX")
export FREELOOP_HOME="$DATA/home" FREELOOP_DEV=1 FREELOOP_DEV_SECRETS=memory WAILS_SERVER_PORT="$PORT"
mkdir -p "$FREELOOP_HOME"
cat > "$FREELOOP_HOME/config.yaml" <<'YAML'
ui:
  onboarded: true
YAML

pid=
cleanup() {
  [ -z "$pid" ] || kill "$pid" 2>/dev/null || true
  rm -rf "$DATA"
}
trap cleanup EXIT INT TERM

(cd "$ROOT/frontend" && npm run build >/dev/null 2>&1)
(cd "$ROOT" && go build -tags server -o "$DATA/freeloop" ./cmd/freeloop) 2>&1 | grep -v -e 'ld: warning' -e '^# ' || true
swiftc -O "$HERE/appcheck/drive.swift" -o "$DATA/drive" 2>&1 | grep -v warning || true

APP="$DATA/freeloop"
"$APP" >"$DATA/app.log" 2>&1 &
pid=$!
i=0
until "$APP" sessions >/dev/null 2>&1; do
  i=$((i + 1))
  [ $i -lt 100 ] || { echo "the app did not start:" >&2; tail -5 "$DATA/app.log" >&2; exit 2; }
  sleep 0.2
done

for name in alpha beta menu; do
  mkdir -p "$DATA/projects/$name"
  git -C "$DATA/projects/$name" init -q
  git -C "$DATA/projects/$name" -c user.name=check -c user.email=check@example.com commit -q --allow-empty -m start
  "$APP" new "$DATA/projects/$name" -title "Check $name" -provider demo -model scripted >/dev/null
done

# Files for the viewer: Markdown carrying markup that must not run, and a PDF.
printf '# A heading\n\nSome **bold** text.\n\n<img src=x onerror="window.pwned=1">\n\n[not a link](javascript:window.pwned=1)\n' > "$DATA/projects/alpha/guide.md"
printf '%%PDF-1.4\0binary\n' > "$DATA/projects/alpha/paper.pdf"

# A session on another source than the demo provider. It never starts its agent here.
"$APP" new "$DATA/projects/beta" -title "Check agent" -profile claude-code >/dev/null

# A long transcript, to open and to follow: turns of the demo, approved from here.
mkdir -p "$DATA/projects/long"
git -C "$DATA/projects/long" init -q
long=$("$APP" new "$DATA/projects/long" -title "Check long" -provider demo -model scripted)
n=0
while [ $n -lt "${APPCHECK_TURNS:-25}" ]; do
  "$APP" prompt "$long" "go" >/dev/null 2>&1 &
  p=$!
  while kill -0 "$p" 2>/dev/null; do
    for r in $("$APP" approvals 2>/dev/null | grep -o 'perm_[0-9_]*'); do "$APP" approve "$r" >/dev/null 2>&1 || true; done
    sleep 0.05
  done
  n=$((n + 1))
done

status=0
"$DATA/drive" "http://localhost:$PORT/" "$HERE/appcheck/scenario.js" || status=$?

# What the page says it saved has to be in the file.
if ! grep -q 'allowBypass: true' "$FREELOOP_HOME/config.yaml"; then
  echo "FAIL  bypass is in config.yaml"
  status=1
else
  echo "ok    bypass is in config.yaml"
fi
[ "$status" -eq 0 ] && echo "appcheck passed" || echo "appcheck FAILED"
exit "$status"
