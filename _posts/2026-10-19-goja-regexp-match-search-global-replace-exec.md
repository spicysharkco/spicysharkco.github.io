---
layout: post
title: "An undefined exec threw: RegExp match, search and global replace in goja"
date: 2026-10-19
project: goja
pr: https://github.com/dop251/goja/pull/808
status: merged
tags: [observability-data, go, testing]
---

Setting `exec` to `undefined` on a regexp made `match`, `search` and a global `replace` throw in goja, a JavaScript engine in Go. [PR #808](https://github.com/dop251/goja/pull/808) fixed it and was merged about two hours and forty minutes after I opened it.

## When it happens

```js
var re = /a/g;
re.exec = undefined;
"bab".replace(re, "X");  // Node: "bXb"
"bab".match(re).join();  // Node: "a"
re = /a/;
re.exec = 1;
"bab".match(re).join();  // Node: "a"
"bab".search(re);        // Node: 1
```

Run on its own, each call throws `TypeError: Value is not an object` in goja.

## How RegExp match, search and replace work

RegExpExec (22.2.7.1) calls `exec` if it is callable, and otherwise uses the built-in matcher. A non-global `match` and `search` call it once (22.2.6.8 step 5, 22.2.6.12 step 6). A global `match` or `replace` loops until a call returns null (22.2.6.8 step 10.a, 22.2.6.11 step 12.a).

## Why the gap existed

A plain regexp takes a fast path that never reads `exec`; assigning an own `exec` ends it. Three generic functions then read `exec` once and required it to be callable: `regexpproto_stdMatcherGeneric` and `regexpproto_stdSearchGeneric` since February 2020, and `getGlobalRegexpMatches`, where the global loop moved in August 2020. A `regExpExec` that falls back has existed since January 2021.

## The fix

One of three hunks in `builtin_regexp.go`, from `regexpproto_stdMatcherGeneric`, trimmed to changed lines, name after `@@` removed:

```diff
@@ -809,6 +805 @@
-	execFn, ok := r.toObject(rx.getStr("exec", nil)).self.assertCallable()
-	if !ok {
-		panic(r.NewTypeError("exec is not a function"))
-	}
-
-	return r.regExpExec(execFn, rxObj, s)
+	return regExpExec(rxObj, s)
```

The other two (`getGlobalRegexpMatches`, `regexpproto_stdSearchGeneric`) change the same way. A non-regexp `this` without a callable `exec` still throws `TypeError`, now with the text `no RegExpMatcher internal slot`.

## The test

`TestRegExpReplaceSplitExec` gains 10 assertions. Five cover the fix: global `replace`, `replaceAll` and `match` with `exec` undefined, and `match` and `search` with `exec = 1`. Three expect `TypeError` from plain objects, as the old code also threw; they pin that it still does. Two count reads of a getter `exec`: global `match` and `replace` of "bab" read it twice: once for the match, once for the null that ends the loop.

3 of 3 runs failed with `TypeError: Value is not an object: undefined` without the change and passed with it. Putting back each hunk alone also failed the test.

## What the maintainer did

@dop251 merged it with a merge commit and no comments. The merged tree matches my commit.

## The lesson

- When a helper does a spec step, grep for functions that still do it by hand. Three read and checked `exec` inline beside `regExpExec`; now only the helper reads it.
- Run each line of a repro on its own. In one script the first throw ends the run, so one throwing line hides the three after it.

If an engine of yours throws where the spec falls back, reply on [X](https://x.com/spicysharkco).
