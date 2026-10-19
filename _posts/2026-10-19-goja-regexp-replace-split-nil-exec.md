---
layout: post
title: "A nil exec was called: generic RegExp replace and split in goja"
date: 2026-10-19
project: goja
pr: https://github.com/dop251/goja/pull/807
status: merged
tags: [observability-data, go, testing]
---

An empty object as `this` made `RegExp.prototype[Symbol.replace]` panic with a nil pointer in goja, a JavaScript engine in Go. So did a non-global `replace` on a regexp with `exec` undefined, and a `split` whose splitter had no `exec`. [PR #807](https://github.com/dop251/goja/pull/807) fixed all three and was merged about 13 minutes after I opened it.

## When it happens

```js
RegExp.prototype[Symbol.replace].call({}, "a", "b"); // Node: TypeError
var r = /a/; r.exec = undefined;
"bab".replace(r, "X");                               // Node: "bXb"
r = /a/;
r.constructor = {[Symbol.species]: function() { return {}; }};
"ab".split(r);                                       // Node: TypeError
```

## How RegExp replace and split work

RegExpExec (22.2.7.1) calls `exec` if callable, else needs a built-in matcher or throws `TypeError`. A non-global `replace` calls it in step 12.a of 22.2.6.11. `split` (22.2.6.14, steps 15.a and 19.b) calls it on a splitter built by the species constructor, not on the receiver.

## Why the gap existed

The generic paths, `regexpproto_stdReplacerGeneric` and `regexpproto_stdSplitterGeneric`, read `exec` with `toMethod` and called the result: `split` since February 2020, `replace` since August 2020. `toMethod` returns nil for `undefined`, so a missing `exec` was called as nil. A `regExpExec` that follows the spec has existed since January 2021.

## The fix

The whole diff to `builtin_regexp.go`, trimmed to changed lines and without function names after `@@`:

```diff
@@ -996 +995,0 @@
-	execFn := toMethod(splitter.ToObject(r).self.getStr("exec", nil)) // must be non-nil
@@ -999 +998 @@
-		if r.regExpExec(execFn, splitter, s) == _null {
+		if regExpExec(splitter, s) == _null {
@@ -1008 +1007 @@
-		z := r.regExpExec(execFn, splitter, s)
+		z := regExpExec(splitter, s)
@@ -1182,2 +1181 @@
-		execFn := toMethod(rxObj.self.getStr("exec", nil)) // must be non-nil
-		result := r.regExpExec(execFn, rxObj, s)
+		result := regExpExec(rxObj, s)
```

A non-regexp `this` with no callable `exec` now throws a `TypeError` with `no RegExpMatcher internal slot`. A non-global `replace` on a regexp with `exec` undefined or not callable, and a `split` whose splitter is one, use the built-in matcher.

## The test

`TestRegExpReplaceSplitExec` has 9 assertions: `replace` on plain objects and on a regexp with `exec` undefined or 1, `split` on a splitter with no `exec`, and a getter that counts reads. Three runs without the fix each hit the panic; `-count=3` with it passed.

## What the maintainer did

@dop251 merged it with a merge commit and no comments. Both files match my commit.

## The lesson

- Count the reads of a property the spec fetches on every call. The old `split` read `exec` once before its loop. A fix that kept the single read passed eight rows and failed the ninth, a getter counting reads (3 for "bab"): `Expected SameValue(«1», «3»)`.
- In `split` the receiver is not the object whose `exec` runs. With `r.exec = undefined`, `RegExp.prototype[Symbol.split].call(r, "a")` returns `["", ""]`, with or without the fix. Set the property on the object the code reads.

If an engine of yours calls a method it never checked for, reply on [X](https://x.com/spicysharkco).
