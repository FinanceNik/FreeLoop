# The lived-in data folder for the app check's second pass (sourced by appcheck.sh):
# settings as a user has them after weeks, not a fresh install's. What the app
# itself must add, a global allow rule, the scenario adds through the page.

# lived_in_files writes the files the app reads at start into the data folder $1.
lived_in_files() {
  home=$1
  mkdir -p "$home"
  # A default profile on an external agent, a default token optimizer (waiting for
  # its approval, as one chosen in a file does), a larger font, a hook written by hand.
  cat > "$home/config.yaml" <<'YAML'
ui:
  onboarded: true
  theme: dark
  language: en
  fontSize: 15
defaults:
  profileId: opencode
  optimizer: rtk
hooks:
  - event: pre_tool_use
    match: shell
    command: "true"
YAML
  cat > "$home/providers.yaml" <<'YAML'
version: 1
providers:
  - id: stand-in
    name: Stand-in
    type: scripted
YAML
  # A server that cannot start.
  cat > "$home/mcp.yaml" <<'YAML'
version: 1
servers:
  - id: broken
    name: Broken server
    transport: stdio
    command: /nonexistent/mcp-server
YAML
  # Dozens of skills with long descriptions, as a Claude Code user's are.
  n=1
  while [ $n -le 40 ]; do
    d="$home/skills/skill-$n"
    mkdir -p "$d"
    cat > "$d/SKILL.md" <<MD
---
name: skill-$n
description: Use this skill whenever you are about to review, design, data, chart, plot, settings, session or anything else that needs careful step $n of a long and thorough process with many words in it, so that a search finds letters everywhere.
---

Do step $n.
MD
    n=$((n + 1))
  done
}

# lived_in_sessions makes sessions in six projects under $2 with the app $1.
lived_in_sessions() {
  app=$1
  data=$2
  i=1
  for p in api web docs infra mobile data; do
    mkdir -p "$data/projects/$p"
    git -C "$data/projects/$p" init -q
    git -C "$data/projects/$p" -c user.name=check -c user.email=check@example.com commit -q --allow-empty -m start
    k=1
    while [ $k -le 5 ]; do
      # The default profile is on an agent that may not be here, so these name theirs.
      "$app" new "$data/projects/$p" -title "Work $i" -profile default -provider demo -model scripted >/dev/null
      i=$((i + 1))
      k=$((k + 1))
    done
  done
}
