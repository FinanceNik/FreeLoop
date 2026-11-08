---
name: grill-the-brief
description: Interview the user about a brief, one question at a time with a suggested answer, until the two of you share one design. Use before writing a PRD or any code for a new feature.
license: Apache-2.0
metadata:
  source: Freeloop's built-in workflow "Plan, slice, run AFK"
---

# Grill the brief

The goal is a shared design, not a document. Nothing is written until the user says the design is settled.

## How to interview

1. Read the brief, then read the code it touches. Never ask what the code can answer: look it up and say what you found.
2. Ask **one question at a time**. Each question comes with the answer you suggest and why, so the user can reply "yes" or correct you.
3. Walk the design tree from the root: what problem this solves and for whom, then the behaviour a user sees, then the data and interfaces, then edge cases and failure, then how it is tested, then what is out of scope.
4. When an answer opens a new branch, follow it before moving on. When an answer contradicts an earlier one, say so and settle it.
5. Keep a short running list of the decisions made. Show it when asked, and before you stop.

## When to stop

Stop when every branch has an answer the user agreed to, or when the user says to stop. End with the list of decisions, and the questions left open, if any. Do not write a PRD or code in this step.
