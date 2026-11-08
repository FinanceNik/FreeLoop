---
title: Numbers, dates, times and costs follow the language
status: open
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
