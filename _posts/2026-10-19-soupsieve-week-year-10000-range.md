---
layout: post
title: "A year limit inherited from datetime: range matching for week inputs in soupsieve"
date: 2026-10-19
project: soupsieve
pr: https://github.com/facelessuser/soupsieve/pull/322
status: merged
tags: [observability-data, python, datetime]
---

In soupsieve, the CSS selector library behind Beautiful Soup, `:in-range` and `:out-of-range` raised `ValueError` for a `week` input with a week-year of 10000 or more. I fixed it in [PR #322](https://github.com/facelessuser/soupsieve/pull/322), merged about 25 minutes after opening.

## When it happens

```py
import soupsieve as sv
from bs4 import BeautifulSoup

soup = BeautifulSoup('<input id="a" type="week" min="10000-W01" max="10000-W52" value="10000-W20">', 'html.parser')
sv.select(':in-range', soup)
```

It raised `ValueError: year 10000 is out of range` instead of returning the input. The HTML spec says a week string's year is "Four or more ASCII digits", so `10000-W20` is valid. Every release from 1.7, where these selectors were added, through 3.0.3 raises. Such years are rare; one extra digit, as in `20255-W01`, is enough.

## How the last week is found

A week-year has 52 or 53 weeks, and `W53` is valid only in a 53-week year. `validate_week()` in `soupsieve/css_match.py` took the last week from the ISO week of `datetime(year, 12, 28)`, a day always in the last week.

## Why the gap existed

The week pattern has taken four or more digits since `:in-range` arrived in [#61](https://github.com/facelessuser/soupsieve/pull/61) in 2019. That code used `strptime`, which also fails past 9999. [#288](https://github.com/facelessuser/soupsieve/pull/288), released in 2.8.2, replaced it with `datetime(year, 12, 28)` to fix end-of-year weeks, with the same limit. No test used a week-year above 9999.

## The fix

The `soupsieve/css_match.py` diff, without context lines:

```diff
@@ -450 +450,5 @@ class Inputs:
-        max_week = datetime(year, 12, 28).isocalendar()[1]
+        #
+        # `datetime` stops at year 9999, but the calendar repeats every 400
+        # years (a whole number of weeks), so use the matching year from 2000
+        # to 2399, as `calendar.weekday()` does.
+        max_week = datetime(2000 + year % 400, 12, 28).isocalendar()[1]
```

400 years are 146097 days, or 20871 whole weeks, so a year and the year 400 later share weekdays and week count. Old and new `validate_week()` agree for every year from 1 to 9999 and weeks 0 to 60.

## The test

Two rows joined each selector's week test. `10000-W53` does not parse, since 10000 has 52 weeks, so it is never out of range and `:in-range` matches it. `10004-W53` parses, since 10004 has 53 weeks, and is above the max `10004-W52`, so `:out-of-range` matches it. With `-k week`, 2 failed before the fix and 2 passed after.

## What the maintainer did

@facelessuser approved and merged it with no change requested, and the code and tests merged as I opened them.

## The lesson

- When a type cannot hold your input, check whether the answer repeats. A year's week count repeats every 400 years, and `calendar.weekday()` folds years the same way.
- A modulo fold needs a base when zero is invalid: `10000 % 400` is 0 and `datetime` has no year 0, so the fix adds 2000. Test a value whose remainder is 0; `10001` would pass without it.

If your parser takes more digits than the type behind it, reply on [X](https://x.com/spicysharkco).
