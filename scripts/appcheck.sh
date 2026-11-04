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

for name in alpha beta; do
  mkdir -p "$DATA/projects/$name"
  git -C "$DATA/projects/$name" init -q
  git -C "$DATA/projects/$name" -c user.name=check -c user.email=check@example.com commit -q --allow-empty -m start
  "$APP" new "$DATA/projects/$name" -title "Check $name" -provider demo -model scripted >/dev/null
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
