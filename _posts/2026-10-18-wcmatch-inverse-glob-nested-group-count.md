---
layout: post
title: "A count zeroed by a nested call: inverse extended globs in wcmatch"
date: 2026-10-18
project: wcmatch
pr: https://github.com/facelessuser/wcmatch/pull/245
status: merged
tags: [observability-data, python, globs]
---

`!(a)@(b|*(c))` raised `re.error` in wcmatch, a Python library for wildcard and glob matching, although Bash accepts it. I fixed it in [PR #245](https://github.com/facelessuser/wcmatch/pull/245), merged about an hour and a half after I opened it. It was my first wcmatch PR.

## When it happens

```python
from wcmatch import fnmatch

fnmatch.fnmatch('xb', '!(a)@(b|*(c))', flags=fnmatch.EXTMATCH)
```

This raised `re.error: missing ), unterminated subpattern at position 5`. `glob.globmatch` with `glob.EXTGLOB` raised it too, and so did release 11.1. The trigger is an inverse group, then an extended group holding another.

## How the cleanup works

A `!(...)` group becomes a lookahead that needs the text after it. The parser appends an `InvPlaceholder` and adds one to `self.inv_ext`. `clean_up_inverse` turns each placeholder into the closing lookahead and returns early when `self.inv_ext` is 0. Among other places, it runs after each nested group and at the end.

## Why the gap existed

A nested group hands the cleanup its parent's list, which holds no placeholder for the `!(a)` before it. The cleanup still ended with `self.inv_ext = 0`, a line from 2018, so the final cleanup saw 0 and left `!(a)` open. The rows from [#219](https://github.com/facelessuser/wcmatch/pull/219) put a plain `@(foo|bar)` after an inverse group, with no group inside it.

## The fix

The reset is gone, and `clean_up_inverse` lowers `self.inv_ext` by one per placeholder it closes. The diff is the whole source change, with one line of context.

```diff
@@ -1540,4 +1540,4 @@ class WcParse(Generic[AnyStr]):
                 )
+                self.inv_ext -= 1
             index -= 1
-        self.inv_ext = 0
 
```

## The test

Five rows go into the existing tables: three in `TestFnMatch.cases`, two in `TestGlobMatch.cases`. Two patterns also get a row that must not match, such as `2_foo` for `!(2)_@(foo|*(bar))`, so matching everything cannot pass.

In those two test files the old source fails the five rows with `re.error` and passes 855 others. The fix passes 860, with 64 skipped. No test covers the decrement: with it reverted, the same 860 pass. `translate()` gave identical regexes with and without it for 15,078 random pattern and flag combinations.

## What the maintainer did

As opened, the PR only deleted the reset. About half an hour in, @facelessuser wrote: "I believe the more appropriate approach is to decrement `self.inv_ext` inside the loop." I did that in a second commit. They merged the PR about 35 minutes after my reply, with the code and tests unchanged.

## The lesson

- A shared count should change only for what the call closed. The nested cleanup found no outer placeholder and still zeroed it.
- After a fix for one nesting shape, vary the shape: nest one level deeper, and swap the inner group's type.
- A change that only keeps state exact may alter no output. Compare output over many inputs and say so; do not claim a test.

If a parser in your own code keeps a count of open groups, reply on [X](https://x.com/spicysharkco).
