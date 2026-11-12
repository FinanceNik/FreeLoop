---
title: A lint rule for effects and their dependencies
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
`App.tsx` and `SessionView.tsx` hold 48 and 46 pieces of state and 56 effects between them, kept right by hand: about eleven "latest value" refs, and one effect with no dependency list that subscribes again on every render (`SessionView.tsx:999`). Nothing checks an effect's dependencies. ESLint with the hooks rules runs in `make lint` (OQ-055, closed 2026-11-11: the user agreed to the three packages).


## Acceptance
- `make lint` runs ESLint over `frontend/src` with `react-hooks/rules-of-hooks` and `react-hooks/exhaustive-deps` as errors, and nothing else turned on.
- Every finding is fixed or carries a one-line reason beside its exemption; the count of exemptions is in the Done note.
- The effect at `SessionView.tsx:999` subscribes once.
- The packages are pinned and development-only; `package-lock.json` changes by them alone.

## Notes
- Three packages at most: `eslint`, `eslint-plugin-react-hooks`, `typescript-eslint`.

## Done
ESLint 10.14.0, eslint-plugin-react-hooks 7.1.1 and typescript-eslint 8.72.2 are pinned in `frontend/lint/` with their own lockfile, not in `frontend/package.json`: typescript-eslint's newest release (8.72.2) accepts TypeScript below 6.1 only and needs the compiler's JavaScript API, which TypeScript 7.0.2 (the app's `tsc`) does not have, so npm refused the install (`ERESOLVE`) and an override did not help. `frontend/package-lock.json` is untouched; the lint folder has TypeScript 6.0.3 for parsing only. `make lint` installs it and runs both hooks rules as errors over `frontend/src`.

The first run found 115 problems. All are fixed: effects that read a callback or value they do not restart on take it through `useEffectEvent` (about 90), a few got the dependency they were missing (`form` in ToolView's key handler, `session.id` in SessionView's draft effects), `useFindings` and `useAgent` (not hooks) were renamed, and the effect at `SessionView.tsx:999` that subscribed on every render now subscribes once. Exemptions: 6, all the same one, a memo keyed by the language (`locale()`), which `t()` reads and the rule cannot see.
