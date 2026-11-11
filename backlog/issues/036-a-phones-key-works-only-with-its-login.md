---
title: A phone's key works only with the login it paired as, and is not a cookie
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
The phone's key is the cookie `freeloop_device`, and a browser sends a cookie to every port of a host. Another service on the same tailnet name, such as a dev server an agent wrote, receives it; replayed to `127.0.0.1:<port>` without Tailscale's login header, the login check is skipped (SEC-020). A device paired with a login is refused without that login, and the key moves out of the cookie.

## Acceptance
- A request with a valid key and no `Tailscale-User-Login` header, for a device paired with one, is refused; so is one with another login; Go tests on a real port, written first.
- The phone app keeps its key where another port's page cannot read it and sends it as a header; the event stream gets a short-lived ticket in place of the cookie. A phone paired before this change pairs again, and the Phone page says so.
- A removed phone's open stream ends at once; Go test.
- SECURITY.md row 22 and the phone's help page say what a sandboxed command on this computer can and cannot reach.

## Notes
- `internal/remote/server.go:328` and `:516`; `internal/remote/stream.go`; `frontend/src/phone/`.
- If a header for the stream needs more than `EventSource` gives, read it with `fetch`; no new dependency.
