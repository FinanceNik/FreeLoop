---
title: A pane that fails leaves the rest of the window working
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
The only error boundary is around the whole window (`main.tsx:20`): one item that fails to draw replaces every pane, the approvals of other sessions with it, and the workspace opens the same session again on reload. Each pane and each page gets its own boundary, and the desktop page gets a Content Security Policy as the phone has.

## Acceptance
- A session made to throw while drawing shows the failure in its pane with "Copy a report" and a way to close it; the other pane, the sidebar and a waiting approval go on working; app check step.
- A reload after such a failure does not put the failing session back in front.
- The desktop page is served with a policy that allows no inline script and no source but the app's own and the preview's localhost frames; app check reads it and all its steps pass under it.
- A non-string image `uri` no longer throws in the fold (`transcript.ts:333`); unit test.

## Notes
- The phone's policy is in `internal/remote`; the desktop's assets come from `frontend/embed.go` through Wails.
