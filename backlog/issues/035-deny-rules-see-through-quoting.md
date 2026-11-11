---
title: A shell deny rule sees through quotes, noglob and the order of flags
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
`simpleCommands` unquotes only the first word, so with `deny git push *` the command `git "push" origin main` is not matched; `noglob git push` and `=git push` get past too, and `rm -fr` past `rm -rf *` (SEC-003). Every word is compared unquoted, zsh's `noglob`, `nocorrect`, `command`, `builtin`, `exec` and `=name` are wrappers, and a command the parser cannot read plainly is never run unasked.

## Acceptance
- With `deny git push *` and `allow git *`: `git "push" origin main`, `git 'push'`, `git pu""sh`, `git p\ush`, `noglob git push`, `=git push` and `command git push` are denied; Go tests in `internal/permission`, written first.
- A deny rule on `rm -rf *` also holds for `rm -fr`, `rm -r -f` and `rm --recursive --force`, or the help page says a rule matches the words as written and shows how to write one that holds; whichever is chosen, a test fixes it.
- A command with quoting or escapes the parser does not resolve asks in a sandboxed environment and in accept-edits mode, and is denied in bypass when any deny rule names its first word.
- `freeloop check` gives the same answers.
- `frontend/src/help/permissions.md` (and the German and Dutch pages) and SECURITY.md say how far a command rule reaches.

## Notes
- `internal/permission/rules.go:194` and `:234`; `plain()` at `internal/permission/broker.go:506`.
- The user's `dev-guard.sh` hook is outside the repository and matches text itself; it is not this issue.
