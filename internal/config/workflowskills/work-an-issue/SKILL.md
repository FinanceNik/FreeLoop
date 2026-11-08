---
name: work-an-issue
description: Do one backlog issue from start to commit, test-first, then mark it done in its file. Use when given an issue from the project's backlog to do, especially unattended.
license: Apache-2.0
metadata:
  source: Freeloop's built-in workflow "Plan, slice, run AFK"
---

# Work an issue

You are given one issue file from the backlog. Do that issue and nothing else.

1. **Read first.** The issue, the PRD it names in `prd:`, the issues it is blocked by, and the code it touches. Read the recent commits to see what the slices before it did.
2. **Test first.** For each acceptance line, write a failing test at the public interface, run it and see it fail for the right reason. Then write the code that makes it pass. Keep each step small.
3. **Check everything.** Run the project's whole test suite and its type checks or linters (look in the Makefile, package.json, go.mod, Cargo.toml or the README for how). Fix what you broke. Do not skip or delete a failing test to get green.
4. **Mark it done.** In the issue file's frontmatter change `status: open` to `status: done`. Change nothing else in it except a short "Done" note at the end if something is worth knowing.
5. **Commit.** Stage your changes and the issue file, and commit with a message that starts with the issue's file name: `003-search-drafts: search drafts by title`.

If you cannot finish (the issue is unclear, a check fails in a way you cannot fix, or you need a person), leave `status: open`, write what is in the way under a "Blocked" heading at the end of the issue file, commit what is safe to commit, and stop. A person reads it next.

Never mark an issue done that is not done: the run trusts that status.
