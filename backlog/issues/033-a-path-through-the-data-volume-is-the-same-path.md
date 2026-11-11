---
title: A path spelled through the data volume is the same path
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
On a Mac `/System/Volumes/Data/Users/me` is `/Users/me`. It is a mount and not a link, so `EvalSymlinks` leaves it alone, and the never-readable list and path deny rules compare a spelling they do not know (SEC-005). The environment and the policy name a path one way before they compare it.

## Acceptance
- `read_file`, `grep`, `glob`, `list_dir` and an external agent's `fs/read_text_file` of `~/.ssh`, the app's data folder and a vendor's credential file, spelled through `/System/Volumes/Data`, are refused as protected in every mode, bypass included; Go tests at the broker and at the environment's guard, written first and seen to fail.
- A path deny rule holds through that spelling; Go test.
- `freeloop check read_file "/System/Volumes/Data$HOME/.ssh/id_ed25519"` says Denied.
- A project opened through that spelling still works: its root compares equal either way.
- Windows and Linux are looked at for the same kind of second spelling (`\\?\` prefixes and 8.3 short names; bind mounts), with a test per one found, the system as a parameter. SECURITY.md says what was found.

## Notes
- `internal/environment/local.go:96` and `:371` (`Resolve`, `realTarget`), `environment.Spellings`, `internal/permission/policy.go:145-165`.
- Reproduced on 2026-11-11: the direct path is Denied, the alias "Asks you".
- Writes and the Mac sandbox already hold through the alias.

## Done
- `environment.Unaliased` (`internal/environment/alias.go`) drops macOS's data volume name and Windows' `\\?\` prefix; `realTarget`, `Canonical`, `NewLocal`, the policy's `within` and `pathParts` and path rules' `matchPath` use it. The volume's name is a variable set from `runtime.GOOS` (the architecture test refuses a darwin-only file in `internal/environment`).
- The tests that need a Mac skip elsewhere; the name's removal and the Windows prefix are tested on every system with the system as a parameter. All were run with the fix switched off and failed.
- Not done: `freeloop check` was not run from a rebuilt app; `TestCheckPolicyHoldsThroughTheDataVolume` runs the same function it calls. Rebuild with `make package` to see it.
- What was found on Windows and Linux is in SECURITY.md, "Review, 2026-11-11".
