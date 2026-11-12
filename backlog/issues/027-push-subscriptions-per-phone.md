---
title: A phone can turn push on, and the computer keeps its subscription
status: done
type: afk
blockedBy: [026-the-phone-app-on-the-home-screen]
prd: sessions-from-the-phone
---

## What
The app makes a VAPID key pair (P-256) once and keeps it in the keychain through `Secrets`. The phone app's Settings gets **Notify me**: on a tap it asks the browser for permission, subscribes with the public key from `GET /api/push/key`, and posts the subscription to `POST /api/push/subscribe`; **Off** unsubscribes (`POST /api/push/unsubscribe`). The subscription is kept with the device in `devices.json`. An endpoint must be `https` on a known push service (`*.push.apple.com`, `fcm.googleapis.com`, `updates.push.services.mozilla.com`), checked before anything else. Settings → Phone shows, per phone, whether it gets notifications.

## Acceptance
- A subscription with an endpoint elsewhere, plain `http`, or a bad key is refused; Go test.
- Subscribing replaces the device's earlier subscription; removing the device removes it; Go test.
- The VAPID key is made once and read from `Secrets` after a restart; Go test with the memory store.
- Local-only mode refuses subscribing; Go test.
- The device list in Settings shows "notifications on"; `make check-app` with a device written into the scratch folder.

## Notes
- No new dependency: `crypto/ecdh`, `crypto/ecdsa`.
- PRD decision 9; SEC-009 changes in issue 028.

## Done
The VAPID key is in `Secrets` as `keychain://freeloop/phone/vapid`; `remote.New` takes the store. Subscribe and key are also refused when the phone server is off; the endpoint check comes first. Sending push, and the service worker handling it, are issue 029.
