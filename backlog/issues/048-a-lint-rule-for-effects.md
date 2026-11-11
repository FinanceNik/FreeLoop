---
title: A lint rule for effects and their dependencies
status: open
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
