---
title: The repository goes public and CI runs green
status: open
type: human
blockedBy: [033-a-path-through-the-data-volume-is-the-same-path, 034-grep-and-glob-ask-before-a-secret-file, 035-deny-rules-see-through-quoting, 036-a-phones-key-works-only-with-its-login, 037-errors-in-the-policy-fail-closed, 038-an-approval-covers-only-what-was-shown, 042-small-questions-stop-reading-whole-transcripts, 045-a-session-not-on-screen-holds-no-transcript, 054-the-documents-say-what-is, 055-windows-and-linux-run-once]
prd: before-going-public
---

## What
The steps in DEVELOPING.md, "Before the repository goes public", each with the user's yes: a last scan for secrets and personal data in the history, private vulnerability reporting turned on, the repository's description and topics, then the switch. A public repository's Actions minutes are free, so the Linux and Windows jobs run for the first time; what they show red is fixed before anything else.

## Acceptance
- `make lint`, `make test` and `make check-app` pass on the commit that goes public.
- The scan finds nothing, or what it finds is dealt with first.
- The first push after the switch runs CI; both jobs end green, or each red test has a fix or an issue the same day.
- The macOS job is started by hand once and is green.
- ROADMAP.md's "From here to 0.1.0" says where things stand, and the freeze of the PRD's decision 1 ends.

## Notes
- After this: tag `v0.1.0-beta.1` and ask three to five testers (ROADMAP.md, stage 4), then the things under "Out of scope" in the PRD.
