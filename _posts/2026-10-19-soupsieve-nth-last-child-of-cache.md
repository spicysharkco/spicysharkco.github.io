---
layout: post
title: "A cached count that left out one element: :nth-last-child(an+b of S) in soupsieve"
date: 2026-10-19
project: soupsieve
pr: https://github.com/facelessuser/soupsieve/pull/320
status: merged
tags: [observability-data, python, caching]
---

In soupsieve, the CSS selector library behind Beautiful Soup, `:nth-last-child(an+b of S)` returned the wrong elements when the first sibling it checked did not match `S`. I fixed it in [PR #320](https://github.com/facelessuser/soupsieve/pull/320), merged under two hours after opening.

## When it happens

```py
import bs4, soupsieve as sv
html = '<ul><li id="0"></li><li id="1" class="x"></li><li id="2" class="x"></li></ul>'
soup = bs4.BeautifulSoup(html, 'html.parser')
print([e['id'] for e in sv.select('li:nth-last-child(1 of .x)', soup)])
```

This printed `['1']`, not `['2']`; with `NOCACHE` it printed `['2']`. Release 2.9.1 was right; every release from 2.10 through 3.0.3 was wrong.

## How the cache counts

`:nth-last-child(an+b of S)` matches an element that matches `S` when its position among the siblings that match `S`, counted from the last, is `an+b`. `match_nth` in `soupsieve/css_match.py` counts from the end once, stores the count in `nth_cache`, and later siblings count down from it.

## Why the gap existed

The cache came in with [#301](https://github.com/facelessuser/soupsieve/pull/301). When the element checked fails `S`, the loop breaks at it without counting it, so the stored count is one short. The one test in `tests/test_level4/test_nth_last_child.py` uses siblings that all match `S`, so no match followed a failing sibling.

## The fix

The diff, with one line of context each side:

```diff
@@ -1110,2 +1110,6 @@ class CSSMatch(_DocumentNav):
                     if child is el:
+                        # Cached "last" lookups count down from `rindex`, so it must be the
+                        # position `el` would have had, even though `el` did not match.
+                        if last:
+                            rindex += 1
                         break
```

In the "last" case the failing element is now counted, so the stored value is the position it would have had. Cached lookups set `last` to false, so the new line runs only on the first lookup from the end.

## The test

One test joined that file: four `p` siblings, only the first without `class="test"`. It checks `1 of .test`, `2 of .test` and `odd of .test` with `util.NOCACHE`, which runs each selector with the cache on and then off. Without the fix the cached run fails.

## What the maintainer did

@facelessuser asked whether `:nth-child` had the same problem. I said only the "last" case does: counting forward leaves a correct count whether or not the element matches. They wrote "Excellent, this was a good catch as we had no tests for this." They asked me to add `lookups`, a word in my new code comment, to the spelling dictionary. I did, in a second commit, and they merged it.

## The lesson

- A cache that stores a loop's count must store what the next lookup expects on every way out of the loop. After each early `break`, ask what a later cache hit does with it.
- Run each assertion with the cache on and off. A selector right uncached and wrong cached puts the fault in the cache; `util.NOCACHE` runs both.

If a cache in your own code stores a loop's count, reply on [X](https://x.com/spicysharkco).
