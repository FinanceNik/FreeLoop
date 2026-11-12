---
title: Sessions from the phone, over the user's own network
status: open
---

## Problem

Someone who leaves several sessions running wants to keep them moving while away from the computer, at the gym or on the road. Today a session that stops to ask for permission waits until they are back, and a session that finished or failed is not noticed until then. Claude Code's remote control solves this by routing the session through its vendor's servers. Freeloop runs no servers, sends nothing anywhere the user did not choose, and keeps every side effect behind its permission broker, so it needs its own way that keeps those promises.

## Solution

The app answers paired phones on a port of the computer itself, and the user's own Tailscale network carries it to the phone, encrypted. A web app served by Freeloop, added to the phone's home screen, shows what needs the user and every session live; from it the user allows or denies a request once, sends a message and stops a turn. The phone is told when a session needs them, finishes or fails, by Web Push sent from the app itself. Everything else, the rules, settings, forms and plans, stays on the computer.

Built so far (2026-11-11, phase 1): the phone server, pairing, the phone app without push, the Settings page, keeping the app alive. Home-screen install is built (issue 026), and so are the VAPID key and a subscription per phone (issue 027). Left: trying it on real phones, push, a QR code, re-confirming approvals, and Windows and Linux.

## User stories

- As someone at the gym, I want to allow the command my session is waiting on from my phone, so that it keeps working while I am away.
- As someone on the road, I want to be told on my phone when a session needs me or finished, so that I don't have to keep checking.
- As someone reading a session on the phone, I want to send it a correction or stop it, so that it doesn't run on the wrong track for an hour.
- As the computer's owner, I want only phones I paired, and confirmed on the computer, to get in, so that no one else on my network reads my sessions.
- As the computer's owner, I want nothing about my sessions to go through anyone's server, so that the phone keeps Freeloop's privacy promise.
- As the computer's owner, I want the transcript to say what was decided or sent from the phone, so that I know afterwards.
- As someone who lost their phone, I want to sign it out from the computer, so that its key is worthless.

## Decisions

1. **The network is the user's own Tailscale (OQ-050).** The app listens on 127.0.0.1 only, and the user runs a `tailscale serve` command the Phone page shows; the app never changes Tailscale's settings itself. No relay, no account with the app. Users without Tailscale wait for OQ-051 (a self-hostable end-to-end relay), a plan of its own.
2. **What the phone may do.** See what needs the user and the sessions live; allow or deny a request once; send a text message; stop a turn. Not: "always", forms, elicitations, plan approvals, rules, settings, terminals, files, attachments or new sessions. The engine decides by an option's kind, looked up on its side, since agents name their options as they like and a form's Send is `allow_once` too.
3. **Pairing.** A one-time code of 100 bits (20 characters), five minutes and five tries, typed or carried in a link (`/phone/#pair=`), and then the user's yes on the computer, which no process can give. The phone's 256-bit token is stored as a hash in `devices.json`, bound to the Tailscale login it paired as; a cookie lasts 90 days and renews on use. A QR code of the link waits for the user's agreement to a library (issue 031).
4. **The record.** A decision or message from the phone keeps the user as its actor and adds `via: "phone:<name>"`; the desktop shows "From the phone (name)". A new actor would break the ACP approval memory.
5. **Requests are checked at the door.** Host must be this computer's address or the name set (no DNS rebinding); Tailscale's Funnel header is refused; posts need the app's header and an Origin of its host; only listed routes exist; bodies are capped; a Content Security Policy comes as a header.
6. **Local-only mode turns the phone off**, since transcripts would leave the computer; so does turning it off, and a file turning it on waits for approval.
7. **The app stays alive.** With the phone on, closing the last window hides it (a Mac) or minimises it; App Nap is held off; on request the computer is kept from idle sleep while a session works or waits. A closed lid still sleeps a laptop unless the system keeps it awake; the app never touches `pmset`.
8. **Sandboxed commands can reach the port.** The Mac sandbox cannot deny one local port while allowing the rest (tried on macOS 27), and Linux's pasta forwards every port. They get nothing without a token and cannot pair; SECURITY.md row 22 says so.
9. **Push is Web Push, sent by the app itself.** A VAPID key in the keychain; payloads encrypted per RFC 8291 and holding only the kind and the session's ID, so Apple's or Google's push service learns nothing of the sessions; endpoints only on a list of known push services; refused in local-only mode; off per phone until the user turns it on there. iOS delivers push only to a web app added to the home screen (iOS 16.4 and later).
10. **The phone app is a separate build** (`vite.phone.config.ts` into `dist/phone`), sharing the conversation reducer, Markdown and the catalogues, with no Wails runtime: links and the clipboard go through `src/host.ts`.

## How it is tested

- **Go, `internal/remote`, against a real engine on a real port** (built): pairing with the code and the user's yes; another host, Funnel, a cross-site post, another Tailscale login and a removed phone refused; a demo request allowed once and "always" refused; `via` on the decision and the message; a session streamed from its start; off in local-only mode. Push adds: subscription per device, the endpoint list, the RFC 8291 test vector, the VAPID token's signature and claims, local-only refusal, no push while the phone's stream is open.
- **Frontend, `node --test`:** the once-only options, pairing links, the phone's name and language; push adds the notification text per kind and language.
- **`make check-app`:** the Phone page turns the server on and off, needs a name before pairing, shows a code and its link (built); push adds the device's push switch in the list.
- **People, on real phones:** an iPhone and an Android phone over cellular: pairing, the home-screen app, approving a real Claude Code request, a push arriving with the window closed.

## Out of scope

- Using the phone without Tailscale (OQ-051).
- Plan approvals, forms, attachments, voice and new sessions from the phone.
- A native app in an app store.
- Push through a service of the app's own, or ntfy.
- Keeping a closed laptop awake against the system's own settings.

## Open questions

- OQ-051: the phone without Tailscale, through a self-hostable relay.
- OQ-052: a passkey to re-confirm approvals after some hours (issue 032 waits on it).
- Issue 031: may the app use `rsc.io/qr` for the pairing QR code?
