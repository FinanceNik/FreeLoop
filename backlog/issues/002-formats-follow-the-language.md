---
title: Numbers, dates, times and costs follow the language
status: done
type: afk
blockedBy: [001-language-setting-and-settings-page]
prd: german-and-dutch-interface
---

## What
Wherever the app writes a number, date, time or cost, it uses the chosen language's style: 1.250 and 18:30
in German, the Dutch equivalents in Dutch, as today in English. Costs stay in US dollars, in the locale's
style (146,51 $).

## Acceptance
- The translation module exposes the locale; every `toLocaleString`, `toLocaleDateString` and `Intl`
  formatter in the frontend takes it from there (no call without a locale).
- The token formatter writes 7.9M in English and the localised form in German and Dutch (unit test).
- The chart axis time labels are 24-hour in German and Dutch and unchanged in English (unit test).
- Cost formatting gives $146.51 in English and 146,51 $ in German (unit test).
- Changing the language re-renders the usage pane's numbers without a reload (`make check-app`).

## Done
- `locale()` is the language in the region the system prefers for it (`en-GB`, `de-AT`), else the language
  alone, so an English Mac keeps its own clock and date order. `numberFormat(options)` caches a formatter per
  locale; a number in a `{placeholder}` goes through it, so English counts gain a comma (1,250), as costs do
  ($1,250.00).
- German has no short form for thousands: the token formatter writes 12.940 there, 7,9 Mio. from a million.
  The context meter in a pane's header keeps k and M (1,2M), since the long form would not fit.
- Left to the views' issues (003, 004), as words rather than formats: weekday names (`activity.ts`,
  `schedules.ts`, `UsageOverview.tsx`), duration units (1h 5m) and relative times (5 min ago). Intl gives
  weekday names in `locale()`. ISO days (2026-11-08) in the Settings cost table stay as they are.
- Found, not changed: with the usage pane the active pane, Back from Settings opens the empty screen rather
  than the workspace (`leaveSettings` takes the pane's id `usage` for a session that is gone). The app check
  activates a session's pane first.
- The AFK step of `make check-app` waited 10 seconds for a run to say why its issue waits, which the engine
  sets on the schedule loop's next pass after queueing the issue, possibly its 30-second tick. The new step
  before it moved that tick out of the window, so it now waits 35 seconds. Kicking the loop again once an
  issue is queued would show the reason at once.
