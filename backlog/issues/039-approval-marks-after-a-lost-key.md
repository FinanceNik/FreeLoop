---
title: A lost approval key approves nothing, and the socket has no guessable name
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
When the keychain item that signs approval marks is gone (a reset keychain, a new computer), the next start adopts everything the files hold, what was waiting included. And on Linux, with a data folder too deep for a socket path, the socket goes to a name in `/tmp` anyone can guess.

## Acceptance
- With the key gone, nothing a file asks for is adopted: hooks, allow rules, `accept_edits`, environments and bypass wait for the user's yes again, and Settings says why; Go test in `internal/engine`, written first.
- The fallback socket is in a folder made for this user with mode 0700 and a random name; Go test on Linux's path logic.
- SECURITY.md row 18 says what marks cover (the YAML files) and what they do not (`devices.json`, project trust in the database, session rules replayed from a transcript), and that on Linux any process of the same user can read the key.

## Notes
- `internal/engine/approvals.go:667`; `internal/engine/rules.go:183`; `internal/config/config.go:436`.
