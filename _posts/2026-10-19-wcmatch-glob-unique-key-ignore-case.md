---
layout: post
title: "A key lowered on lookup but stored as is: unique glob results in wcmatch"
date: 2026-10-19
project: wcmatch
pr: https://github.com/facelessuser/wcmatch/pull/247
status: merged
tags: [observability-data, python, globs]
---

In wcmatch, a Python glob and fnmatch library, `glob.glob` and `glob.iglob` returned a path twice when two patterns found it, `IGNORECASE` was set and the path had an upper case letter. I fixed it in [PR #247](https://github.com/facelessuser/wcmatch/pull/247), merged about eight minutes after opening.

## When it happens

```py
from wcmatch import glob

# the directory holds a file named Ab
glob.glob(['A*', '*b'], flags=glob.IGNORECASE)
# ['Ab', 'Ab']   want ['Ab']
```

The docs say `glob` and `iglob` return unique paths. Releases 8.1.1 through 11.1 return the path twice for this call; 8.0, 8.0.1 and 8.1 return it once.

## How the unique check works

`Glob._is_unique` in `wcmatch/glob.py` accepts a path only if its key is not in the set `self.seen`. The lookup key was `path.lower()` when `self.case_sensitive` was false, else `path`; the stored key was always `path`. `self.case_sensitive` says how patterns match: `IGNORECASE` clears it.

## Why the gap existed

Release 6.0.0 added unique results with the same store and a lookup that lowered the path when matching was case sensitive. [#165](https://github.com/facelessuser/wcmatch/pull/165), in 8.1.1, reversed that condition; its changelog line says the unique cache "had inverted logic". It left the store alone and added no test. In the glob tests, `glob.I` appears in only two places; in neither did two patterns find one path. The unique rows held names like `a` and `aab`, which `lower()` leaves alone.

## The fix

The lookup and the store now share one key, set by the path case rule. The `wcmatch/glob.py` diff, without context lines:

```diff
@@ -817,2 +817,4 @@ class Glob(Generic[AnyStr]):
-        if (path.lower() if not self.case_sensitive else path) not in self.seen:
-            self.seen.add(path)
+        # Paths that only differ by case are the same file only when the file system ignores case.
+        key = path if _wcparse.is_case_sensitive(self.flags) else path.lower()
+        if key not in self.seen:
+            self.seen.add(key)
```

`is_case_sensitive` asks `os.path.normcase`, unless `FORCEWIN` or `FORCEUNIX` is set. The match flag changes only pattern matching, so `a` and `A` stay two files where the rule keeps case. With `IGNORECASE`, `glob.glob('*')` could drop one of them, by listing order; it now returns both.

## The test

One row joined `Testglob.cases`: `{E*,*F}` with `IGNORECASE` expects only the existing file `EF`. Two more classes reuse the table; without the fix the row fails in all three. `TestCase.test_case_unique` expects both `a` and `A` for `*` and `['a*', 'A*']`, and skips itself, like `test_case`, when the file system holds only one of them.

## What the maintainer did

@facelessuser approved and merged it unchanged.

## The lesson

- Compute a set's key once and use it for the lookup and the store. #165 changed only the lookup line.
- Test a key function with input it changes: `lower()` alters only names with an upper case letter.
- Ask what a key stands for: "the same file", which the path case rule decides, not a pattern flag.

If a set of yours is read with one key and written with another, reply on [X](https://x.com/spicysharkco).
