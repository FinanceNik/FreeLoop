---
title: A phone's key works only with the login it paired as, and is not a cookie
status: done
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

## Done
- Server (`internal/remote/server.go`): the key is read from `Authorization: Bearer`; `pairAnswer` returns it once in its JSON and sets no cookie; a device with a login needs that login on every request; a key unused for 90 days (`keyLife`) is refused; `authed` ties each request to a cancel that `RemoveDevice` fires.
- Phone app (`frontend/src/phone/api.ts`): the key is in `localStorage`, sent as a header on every call, cleared on a 401 or a sign-out; the streams are read with `fetch` and the new `parseSSE` (so no ticket was needed), resumed from `Last-Event-ID` after a pause (`reconnectDelay`).
- Not done: the Phone page in Settings does not list "paired before this build" phones separately; there were none in the user's data folder. The phone shows its pairing screen when its key is missing or refused, and the help pages say a phone from an earlier build pairs again.
- Not run on a real phone (issue 025 does): the stream over `fetch` through `tailscale serve`, Safari's Add to Home Screen storage (it has its own `localStorage`, so an installed app pairs on its own).
