---
title: An error while reading the policy asks or denies, never allows
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
Three places let a call through when something could not be read (SEC-003): with unreadable rules an in-project `.env` read is allowed unasked and a plan-mode write becomes a question in place of a denial; an error looking up the project or profile drops their guard hooks silently; a session with no runtime runs no hooks.

## Acceptance
- With `rules.yaml` unreadable: a read of `.env` asks, a write in plan mode is denied; Go tests, written first.
- A project or profile that cannot be looked up makes the call ask, with the reason, as a failing hook does.
- A call for a session without a runtime is judged with its hooks, or asks.
- The log names each of these once, without the path's contents.

## Notes
- `internal/permission/broker.go:458-462`; `internal/engine/hooks.go:70-90` and `:212`.
