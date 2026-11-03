#!/bin/sh
# HRN-007: runs the HRN-005 scenario with each built-in preset's real agent, at
# the version it pins, on a local stand-in for its model. Needs no key and sends
# no prompt to a vendor; it downloads the agents (about 900 MB) into a folder of
# their own and gives each run a scratch home, so no login of yours is read.
# Rerun it after changing a pinned version in internal/config/presets.go.
set -eu
cd "$(dirname "$0")/.."
dir="${FREELOOP_AGENTS_DIR:-${TMPDIR:-/tmp}/freeloop-agents}"
goose_version="1.57.0"

pinned() { # the npm package@version a preset names
	grep -oE "\"$1@[0-9][^\"]*\"" internal/config/presets.go | tr -d '"'
}
npm_agent() { # folder, package
	spec="$(pinned "$2")"
	mkdir -p "$dir/$1"
	(cd "$dir/$1" && npm install --silent --no-audit --no-fund --ignore-scripts "$spec")
}
npm_agent gemini @google/gemini-cli
npm_agent codex @agentclientprotocol/codex-acp
npm_agent opencode opencode-ai
if [ ! -x "$dir/goose/goose" ] || ! "$dir/goose/goose" --version | grep -q "$goose_version"; then
	mkdir -p "$dir/goose"
	curl -fsSL -o "$dir/goose/goose.tar.bz2" "https://github.com/block/goose/releases/download/v$goose_version/goose-$(uname -m | sed s/arm64/aarch64/)-apple-darwin.tar.bz2"
	# Pinned by hash in scripts/agents.sha256; this program is about to run. A new
	# version or platform is added on purpose, by running once with TRUST_NEW_AGENT=1.
	sum=$(shasum -a 256 "$dir/goose/goose.tar.bz2" | cut -d' ' -f1)
	entry="goose $goose_version $(uname -m)"
	pinned=$(grep "^$entry " scripts/agents.sha256 2>/dev/null | cut -d' ' -f4)
	if [ "$sum" != "$pinned" ]; then
		if [ -z "$pinned" ] && [ -n "${TRUST_NEW_AGENT:-}" ]; then
			echo "$entry $sum" >> scripts/agents.sha256
			echo "pinned $entry $sum; commit scripts/agents.sha256"
		else
			echo "goose $goose_version has checksum $sum, expected ${pinned:-none pinned}; see scripts/verify-agents.sh" >&2
			rm -f "$dir/goose/goose.tar.bz2"
			exit 1
		fi
	fi
	tar xjf "$dir/goose/goose.tar.bz2" -C "$dir/goose"
fi
FREELOOP_AGENTS_DIR="$dir" go test ./internal/harness/acp/ -count=1 -v -run '^TestPresetScenarios$' -timeout 30m | grep -E '^ *(---|ok|FAIL|PASS)|_test.go:[0-9]+: (allowed|denied|plan mode|the conversation)'
