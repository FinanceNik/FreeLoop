#!/bin/sh
# Drives the real interface on the real engine in a real WebKit view, on scratch data,
# with the demo provider: what the unit tests cannot see. Bugs it was written for lived
# only in WebKit or between the page and the engine: confirm() answering no, the view
# no longer following an agent, a setting that never saved. The scenario is in
# appcheck/scenario.js. A second pass runs appcheck/lived-in.js on a data folder set up
# as a user's is after weeks (appcheck/lived-in.sh), since some bugs show only there.
# Both share appcheck/lib.js. Needs no key and leaves the login keychain alone.
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
cat > "$FREELOOP_HOME/providers.yaml" <<'YAML'
version: 1
providers:
  - id: stand-in
    name: Stand-in
    type: scripted
YAML
# The profile an AFK run's sessions take in the workflow's step (WFL-005): the default
# profile names no provider here.
mkdir -p "$FREELOOP_HOME/profiles"
cat > "$FREELOOP_HOME/profiles/afk-demo.yaml" <<'YAML'
name: AFK demo
provider: demo
model: scripted
YAML

. "$HERE/appcheck/lived-in.sh"

pid=
# What a run leaves to read afterwards: the app's log, the scenario's lines and a picture
# of each failed check. CI uploads it when the check fails.
OUT=${APPCHECK_OUT:-$ROOT/bin/appcheck}
rm -rf "$OUT"
mkdir -p "$OUT/shots"
export APPCHECK_SHOTS="$OUT/shots"
cleanup() {
  [ -z "$pid" ] || kill "$pid" 2>/dev/null || true
  cp "$DATA/app.log" "$OUT/app.log" 2>/dev/null || true
  cp "$DATA/lived-in.log" "$OUT/app-lived-in.log" 2>/dev/null || true
  rm -rf "$DATA"
}
trap cleanup EXIT INT TERM

(cd "$ROOT/frontend" && npm run build >/dev/null 2>&1)
(cd "$ROOT" && go build -tags server -o "$DATA/freeloop" ./cmd/freeloop) 2>&1 | grep -v -e 'ld: warning' -e '^# ' || true
swiftc -O "$HERE/appcheck/drive.swift" -o "$DATA/drive" 2>&1 | grep -v warning || true

APP="$DATA/freeloop"
# start_app starts the app on $FREELOOP_HOME, logging to $1, and waits for it.
start_app() {
  "$APP" >"$1" 2>&1 &
  pid=$!
  i=0
  until "$APP" sessions >/dev/null 2>&1; do
    i=$((i + 1))
    [ $i -lt 100 ] || { echo "the app did not start:" >&2; tail -5 "$1" >&2; exit 2; }
    sleep 0.2
  done
}
start_app "$DATA/app.log"

for name in alpha beta menu full plan workflow; do
  mkdir -p "$DATA/projects/$name"
  git -C "$DATA/projects/$name" init -q
  git -C "$DATA/projects/$name" -c user.name=check -c user.email=check@example.com commit -q --allow-empty -m start
  # The workflow's agents and the app commit there, whatever this Mac's own git identity.
  [ "$name" != workflow ] || { git -C "$DATA/projects/$name" config user.name check; git -C "$DATA/projects/$name" config user.email check@example.com; }
  # "Check full" sets a model's limit, which needs a provider in the file.
  provider=demo
  [ "$name" != full ] || provider=stand-in
  # "Check plan" starts in plan mode, for the plan's review.
  mode=ask
  [ "$name" != plan ] || mode=plan
  "$APP" new "$DATA/projects/$name" -title "Check $name" -provider "$provider" -model scripted -mode "$mode" >/dev/null
done

# Files for the viewer: Markdown carrying markup that must not run, and a PDF.
printf '# A heading\n\nSome **bold** text.\n\n<img src=x onerror="window.pwned=1">\n\n[not a link](javascript:window.pwned=1)\n' > "$DATA/projects/alpha/guide.md"
# A hostile PDF: script on opening, and a link over the whole page. To the app's own address,
# since the app's policy lets localhost into a frame for the preview, so only the viewer's
# stops this one, and since it really loads, so a click that got through would show.
cat > "$DATA/projects/alpha/paper.pdf" <<PDF
%PDF-1.5
1 0 obj<</Type/Catalog/Pages 2 0 R/OpenAction<</S/JavaScript/JS(app.alert('ran'); app.launchURL('https://example.com/from-js');)>>>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 400 300]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>/Annots[<</Type/Annot/Subtype/Link/Rect[0 0 400 300]/Border[0 0 0]/A<</S/URI/URI(http://localhost:$PORT/)>>>>]>>endobj
4 0 obj<</Length 40>>stream
BT /F1 20 Tf 20 150 Td (HOSTILE PDF) Tj ET
endstream endobj
5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
trailer<</Root 1 0 R>>
PDF
printf '<html><script>window.top.pwned=1</script>' > "$DATA/projects/alpha/fake.pdf"

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
# APPCHECK_SCENARIO runs another scenario on the same data, to try one step alone.
cat "$HERE/appcheck/lib.js" "${APPCHECK_SCENARIO:-$HERE/appcheck/scenario.js}" > "$DATA/fresh.js"
"$DATA/drive" "http://localhost:$PORT/" "$DATA/fresh.js" > "$OUT/results.txt" || status=$?
cat "$OUT/results.txt"

# What the page says it saved has to be in the file.
if ! grep -q 'allowBypass: true' "$FREELOOP_HOME/config.yaml"; then
  echo "FAIL  bypass is in config.yaml"
  status=1
else
  echo "ok    bypass is in config.yaml"
fi

# The second pass: the same app on a lived-in data folder.
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
pid=
export FREELOOP_HOME="$DATA/lived-in/home"
lived_in_files "$FREELOOP_HOME"
start_app "$DATA/lived-in.log"
lived_in_sessions "$APP" "$DATA/lived-in"
echo "-- lived-in"
cat "$HERE/appcheck/lib.js" "$HERE/appcheck/lived-in.js" > "$DATA/lived-in.js"
"$DATA/drive" "http://localhost:$PORT/" "$DATA/lived-in.js" > "$OUT/results-lived-in.txt" || status=1
cat "$OUT/results-lived-in.txt"

[ "$status" -eq 0 ] && echo "appcheck passed" || echo "appcheck FAILED"
exit "$status"
