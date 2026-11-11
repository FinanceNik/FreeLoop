# Your phone

Follow your sessions from your phone while you are away, over your own private network.

## What the phone can do

- See what needs you, and every session, live.
- Allow or deny a request once. Forms, plans, rules and settings wait for the computer.
- Send a message to a session, or stop its turn.

Messages and decisions from the phone say so in the session, with the phone's name.

## Set it up

1. In **Settings → Phone**, turn on **Answer my phone**. Freeloop then answers on a port of this computer only, and keeps running when its window closes.
2. Install Tailscale on this computer and on your phone, sign both in to the same account, and turn on HTTPS certificates in Tailscale's admin console.
3. Run the command the page shows once, in a terminal. It serves Freeloop on your tailnet only. The page also shows how to stop it.
4. Choose **Pair a phone…**, open the address it shows on your phone, and confirm on the computer.

On an iPhone, **Add to Home Screen** in Safari gives it its own icon; if it asks to pair again there, do so.

## Keeping it reachable

- {{The Mac must stay awake. **Keep this computer awake while a session works or waits for me** stops it sleeping when idle; a closed lid still sleeps it unless it is on power with an external display.|The computer must stay awake. **Keep this computer awake while a session works or waits for me** stops it sleeping when idle; closing the lid may still sleep it.|The computer must stay awake. **Keep this computer awake while a session works or waits for me** asks the system not to sleep when idle; closing the lid follows its own setting.}}
- Freeloop must keep running: closing its window hides it while the phone is on. Quitting stops it.
- Local-only mode turns the phone off, since your sessions would leave this computer.

## What stays private

- Freeloop answers only on this computer. Tailscale carries it to your phone, encrypted, and no one else's device can reach it.
- Only phones you paired, and confirmed on the computer, get in. **Sign out every phone** on the page ends them all, and the streams they have open, at once.
- A phone's key is kept in the phone app's own storage and sent as a header, never as a cookie, so another page on the same Tailscale name never receives it, and it stops working under another Tailscale login or after 90 days unused. A phone paired with an earlier build is asked to pair again.
- Commands that agents run can reach the port, like any on this computer, but get nothing there without a paired phone.
- Tailscale lists HTTPS certificates publicly, so the computer's Tailscale name becomes public.
