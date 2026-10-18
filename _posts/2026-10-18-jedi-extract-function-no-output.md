---
layout: post
title: "An assignment to no name: extract_function in Jedi"
date: 2026-10-18
project: Jedi
pr: https://github.com/davidhalter/jedi/pull/2122
status: merged
tags: [observability-data, python, refactoring]
---

`Script.extract_function` in Jedi, a Python library for editor autocompletion and refactoring, wrote code that does not parse when the extracted lines define no name. I fixed it in [PR #2122](https://github.com/davidhalter/jedi/pull/2122), merged about five and a half hours after I opened it.

## When it happens

Extract the two `print` lines:

```python
import jedi

code = "def f(a):\n    print(a)\n    print(a + 1)\n    return a\n"
script = jedi.Script(code)
refactoring = script.extract_function(2, 4, new_name='g', until_line=3)
```

The new code was:

```python
def g(a):
    print(a)
    print(a + 1)
    return 


def f(a):
     = g(a)
    return a
```

` = g(a)` is a syntax error, and `g` ends in a bare `return`. A name assigned but never read worked: Jedi returned it.

## How the return value is chosen

`extract_function` keeps the names the lines define and later code reads. If none are read, a comment says it returns the last defined name. The result goes into `output_var_str`, which feeds the new function's `return` line and the call-site assignment.

## Why the gap existed

The code for statements went live in February 2020 with `or [return_variables[-1]]`, which raises on an empty list. Three days later `if return_variables else []` guarded it. That left the list empty for the two lines that use `output_var_str`, which nothing guarded. No older test expects a bare call when extracting statements.

## The fix

Add the `return` line and the assignment only when `return_variables` is not empty. Otherwise call it as a plain statement. The diff is trimmed: `extract.py` only, no context lines, no function names in hunk headers.

```diff
@@ -250 +250,2 @@
-            code_block += 'return ' + output_var_str + '\n'
+            if return_variables:
+                code_block += 'return ' + output_var_str + '\n'
@@ -285 +286 @@
-        else:
+        elif return_variables:
@@ -286,0 +288,2 @@
+        else:
+            replacement = function_call + '\n'
```

## The test

The case `with-range-func-no-output` in `test/refactor/extract_function.py` extracts two `print` calls and expects a `func` with no `return` and a plain `func(v1)` call. It failed three of three runs before the fix and passed three of three after. Undoing either change made it fail again.

## What the maintainer did

@davidhalter merged it without a review comment or a change to my commit. It was my first Jedi PR.

## The lesson

- A fallback that picks the last item needs an empty-list answer, and so does the code below it. Here a guard stopped the failure; the lines after it still assumed a name.
- Name the trigger from the code that fills the variable. "No name is read later" is wrong: the fallback returns an unread name, and `return_variables` is empty only when no name is defined.
- A bare `return` is valid Python, so the call-site change alone makes the output parse. Compare the text in the test, or a parse check passes with half the fix.

If a refactoring of yours builds text from a list that can be empty, reply on [X](https://x.com/spicysharkco).
