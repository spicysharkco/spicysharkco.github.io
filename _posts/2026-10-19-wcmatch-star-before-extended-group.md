---
layout: post
title: "A group opener consumed as a repeated star: star groups in wcmatch"
date: 2026-10-19
project: wcmatch
pr: https://github.com/facelessuser/wcmatch/pull/246
status: merged
tags: [observability-data, python, globs]
---

In wcmatch, a Python glob and fnmatch library, a star directly before an extended star group, as in `a**(b)`, was not read as `*` followed by the group `*(b)`. I fixed it in [PR #246](https://github.com/facelessuser/wcmatch/pull/246), merged about half an hour after opening.

## When it happens

```py
from wcmatch import fnmatch, glob
fnmatch.fnmatch('abb', 'a**(b)', flags=fnmatch.EXTMATCH)
# False   want True
glob.globmatch('b', '**(a|b)b', flags=glob.GLOBSTAR | glob.EXTGLOB)
# False   want True
```

Bash reads `**(b)` as `*` followed by the group `*(b)`. Release 11.1 returned False for both, and `a**(b)` matched only names ending in the literal text `(b)`.

## How wcmatch reads a run of stars

`WcParse` in `wcmatch/_wcparse.py` reads a pattern in order. With `GLOBSTAR`, a `**` at the start of a segment may be a globstar; with `GLOBSTARLONG`, a third star makes `***`. A loop then collapses the remaining stars into one, because `**` and `*` match the same text.

## Why the gap existed

All three places took the star that opens the group as part of the run. Before 11.0 the loop ran only at the start of a segment, so `a**(b)` worked up to 10.2.1. No file under `tests` held a `**(` pattern.

## The fix

When extended groups are on, each place now stops at a star followed by `(`, so that star stays for the group. The `wcmatch/_wcparse.py` diff, without context lines:

```diff
@@ -1439 +1439 @@ class WcParse(Generic[AnyStr]):
-                if c != '*':
+                if c != '*' or (self.extend and i.match(RE_EXT_GROUP, False)):
@@ -1448 +1448 @@ class WcParse(Generic[AnyStr]):
-                    if c != '*':
+                    if c != '*' or (self.extend and i.match(RE_EXT_GROUP, False)):
@@ -1494 +1494 @@ class WcParse(Generic[AnyStr]):
-            # Consume duplicate starts
+            # Consume duplicate starts, but leave a star that opens an extended group
@@ -1497 +1497 @@ class WcParse(Generic[AnyStr]):
-                while c == '*':
+                while c == '*' and not (self.extend and i.match(RE_EXT_GROUP, False)):
```

`**(x)` now compiles like `*` followed by `*(x)`, so `**(a|aa)c` can backtrack on a long non-matching name, as `*+(a|aa)c` already does.

## The test

Eight rows joined existing tables: three in `TestFnMatch.cases` and five in `TestGlobMatch.cases`, two of them with `glob.GL`, such as `***(a|b)b`. Without the fix, `pytest tests/test_fnmatch.py tests/test_globmatch.py` gave 8 failed and 866 passed; with it, 874 passed.

## What the maintainer did

@facelessuser wrote "Another good find." and squash-merged it. The diff did not change.

## The lesson

- A loop that collapses repeated characters must stop where the next character changes meaning. A `*` before `(` opens a group; it is not a repeat.
- For a regression, name the last release that worked and what the next one widened. Here a loop limited to the start of a segment before 11.0 now runs elsewhere.
- A fix that compiles a pattern like a known one inherits its costs. Say so: `**(a|aa)c` can now backtrack as `*+(a|aa)c` does.

If a parser of yours collapses repeated characters, reply on [X](https://x.com/spicysharkco).
