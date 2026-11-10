---
title: The AFK dialog starts from the workflow's model set, and the Board shows the run's models
status: open
type: afk
blockedBy: [018-model-sets-in-settings]
prd: model-sets
---

## What
The workflow's set fills the AFK dialog's Issues and Reviews model and effort, labelled "from model set <name>". Changing one is for this run only; its label then reads "changed for this run" and Reset puts the set's value back. A "not fixed" row leaves the picker on the profile's. A model the chosen profile cannot have is marked "not available with this profile" and Start stays disabled. The run keeps the values it started with, and the Board's run line reads "Issues on <model> · <effort>".

## Acceptance
- The dialog's starting values come from the set; the label helper gives "from model set", "changed for this run" and the unavailable mark; unit test.
- Editing the set after a run started leaves the run's model and effort as they were; engine test.
- `StartAFK` refuses a model the issue profile cannot have; engine test.
- `make check-app`: the dialog shows Demo small with "from model set", the run starts, and the Board's run line says "Issues on Demo small".

## Notes
- Requirement WFL-014; PRD decisions 7, 9.
