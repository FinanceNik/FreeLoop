---
title: The phone is told when a session needs the user, finished or failed
status: open
type: afk
blockedBy: [028-encrypted-push-from-the-computer]
prd: sessions-from-the-phone
---

## What
The engine's notices (approval, finished, error, budget) are teed in `cmd/freeloop/main.go` to the phone server, which pushes them to every device with notifications on, following the same switches and muted projects as the desktop's notifications, and not to a device whose live stream is open. The service worker shows the notice in the phone's language ("A session needs you", "A session finished", "A session stopped with an error"), with the session's title fetched from the server when it answers, and opens the session on a tap. The home-screen icon shows how many requests wait.

## Acceptance
- An approval in a demo session pushes once to a subscribed device and not to one whose stream is open; Go test with a fake push service.
- Notifications turned off, or the project muted, push nothing; Go test.
- The worker's text per kind and language; unit test.
- A tap opens `/phone/#session=<id>` and the app shows that session; unit test of the route.

## Notes
- The desktop shows its own notices only when no window has focus; the phone gets them whenever its stream is closed.
