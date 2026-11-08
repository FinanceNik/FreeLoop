---
name: write-a-prd
description: Write a product requirements document from a settled design conversation, as a Markdown file in the project's backlog. Use after grilling a brief.
license: Apache-2.0
metadata:
  source: Freeloop's built-in workflow "Plan, slice, run AFK"
---

# Write a PRD

Write the PRD from what was decided in this conversation. Do not invent decisions; where something was left open, list it under "Open questions".

## Where

One file, `<backlog>/prds/<short-name>.md`, where `<backlog>` is the folder the workflow names (`backlog` unless told otherwise) and `<short-name>` is lowercase words joined by dashes.

## Format

```markdown
---
title: <one line>
status: open
---

## Problem
Who has it, and what it costs them today.

## Solution
What changes, as the user will see it.

## User stories
- As a <who>, I want <what>, so that <why>.

## Decisions
Each decision made while grilling, with its reason.

## How it is tested
The behaviour each test checks, at the level of the public interface. Prefer few, deep modules with small interfaces, tested from outside.

## Out of scope
What this does not do, so later work does not assume it.

## Open questions
```

Keep it short enough to read in five minutes. Name modules and interfaces, not file paths that will change.
