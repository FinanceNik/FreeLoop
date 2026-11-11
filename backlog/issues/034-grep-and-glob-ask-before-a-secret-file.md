---
title: Grep and glob ask before they show a secret file
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
Reading `.env` asks (SEC-017), but the check sees only the paths a request names, and `grep` and `glob` name a folder. `grep {pattern: ".", include: ".env"}` returns the file's contents in ask mode with no question. The search itself leaves out files with a secret's name unless the request was allowed for them.

## Acceptance
- A grep over a folder holding `.env` returns no line of it and says that secret files were left out; Go test at the built-in tool and at `environment.Local.Grep`, written first.
- A grep that names the secret file, or includes only such files, asks as a read of it does; allowed, it returns the lines.
- `glob` and `list_dir` still list the file's name.
- The same for an external agent's search that goes through the environment.
- The secret names are the ones a read uses, from one list.

## Notes
- `internal/permission/broker.go:515`; `internal/environment/local.go:539-632` (`walk`, `Grep`); `internal/harness/builtin/tools.go:252` (`hidden`).

## Done
- `environment.SkipSecrets` (a `GrepOption`, so `FileSystem.Grep` takes options) leaves the files of `SecretPath` out and says which; `grep` in `tools.go` uses it unless the path or the include names a secret (`secretInclude`), when it asks as a read does, or the decision is a rule's or bypass's. The note in the result lists up to ten files.
- `glob` and `list_dir` were not changed; a test says they still list the name.
- Not applicable: an external agent's search runs in its own process; what reaches the app is `fs/read_text_file` of a named path, which the broker already asks about.
- The help pages (English, German, Dutch) and SECURITY.md say it.
