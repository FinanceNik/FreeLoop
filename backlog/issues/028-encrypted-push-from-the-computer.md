---
title: The computer sends an encrypted push
status: done
type: afk
blockedBy: [027-push-subscriptions-per-phone]
prd: sessions-from-the-phone
---

## What
`internal/remote/push.go` sends a Web Push message to a device's subscription: the payload encrypted per RFC 8291 (aes128gcm, RFC 8188 framing), with a VAPID authorization (RFC 8292) signed ES256 as raw r‖s, `aud` the endpoint's origin, `exp` within 12 hours and `sub` the project's URL. It goes through `http.DefaultTransport`, so local-only mode blocks it. A 404 or 410 from the push service drops the subscription. The payload is `{"kind","sessionId"}` and nothing else.

## Acceptance
- Encrypting with RFC 8291's example keys and salt gives its example ciphertext; Go test.
- The VAPID token verifies with the public key and carries the claims above; Go test.
- Against a fake push service (`httptest`, allowed in the test): the request has `TTL`, `Content-Encoding: aes128gcm`, the authorization; a 410 drops the subscription; Go test.
- Local-only mode sends nothing; Go test.
- SEC-009, SECURITY.md's outbound row and threat row 22, and the privacy help in English, German and Dutch name the push services; `help.test.ts` passes.

## Notes
- No new dependency: `crypto/hkdf`, `crypto/aes`, `crypto/cipher`, `crypto/ecdsa`.
- Apple refuses a missing or invalid `sub`.

## Done

The sending is in `internal/remote/pushsend.go` (`Server.Push(deviceID, kind, sessionID)`), next to `push.go`. The worker shows English, German or Dutch text for the kinds `approval`, `done` and `failed` and a generic line for any other; issue 029 sets the kinds the engine sends and should match them there. Tapping a notification opens `/phone/#session=<id>`; the phone app does not read that link yet.
