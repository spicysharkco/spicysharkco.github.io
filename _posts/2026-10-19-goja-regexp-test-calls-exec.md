---
layout: post
title: "A method that never read exec: RegExp.prototype.test in goja"
date: 2026-10-19
project: goja
pr: https://github.com/dop251/goja/pull/806
status: merged
tags: [observability-data, go, testing]
---

`RegExp.prototype.test` in goja, a JavaScript engine in Go, never read `exec`. A plain object with `exec` threw, and a replaced `exec` was ignored. I fixed it in [PR #806](https://github.com/dop251/goja/pull/806), merged about an hour after I opened it.

## When it happens

```js
RegExp.prototype.test.call({exec() { return null; }}, "a");  // TypeError, want false
var r = /a/; r.exec = () => null; r.test("a");               // true, want false
RegExp.prototype.exec = () => null; /a/.test("a");           // true, want false
```

## How RegExp.prototype.test works

The spec (ECMA-262 22.2.6.16) calls `RegExpExec` (step 4 in the current draft), which reads `exec` and calls it if callable (22.2.7.1). Only otherwise does the built-in matcher run. goja's `regexpproto_test` required a `*regexpObject` and always ran the built-in matcher.

## Why the gap existed

`regexpproto_test` has had the same logic since November 2016. `Symbol.match` and `Symbol.search` got `checkStdRegexp`, which accepts only an unmodified regexp, and a path through `exec` in February 2020. `test` was not moved over.

## The fix

The whole diff to `builtin_regexp.go`, with one line of context:

```diff
@@ -487,4 +487,6 @@ func (r *Runtime) regexpproto_exec(call FunctionCall) Value {
 func (r *Runtime) regexpproto_test(call FunctionCall) Value {
-	if this, ok := r.toObject(call.This).self.(*regexpObject); ok {
-		if this.test(call.Argument(0).toString()) {
+	thisObj := r.toObject(call.This)
+	s := call.Argument(0).toString()
+	if this := r.checkStdRegexp(thisObj); this != nil {
+		if this.test(s) {
 			return valueTrue
@@ -493,5 +495,7 @@ func (r *Runtime) regexpproto_test(call FunctionCall) Value {
 		}
-	} else {
-		panic(r.NewTypeError("Method RegExp.prototype.test called on incompatible receiver %s", r.objectproto_toString(FunctionCall{This: call.This})))
 	}
+	if regExpExec(thisObj, s) == _null {
+		return valueFalse
+	}
+	return valueTrue
 }
```

If `checkStdRegexp` accepts the regexp, the built-in matcher runs and builds no match array. Otherwise the existing `regExpExec` calls `exec`: `null` is `false`, an object `true`. An object that is not a regexp and has no callable `exec` still throws a `TypeError`, now with the message `no RegExpMatcher internal slot`.

## The test

`TestRegExpTestCallsExec` has 14 assertions. A plain object with `exec` is checked for its `this`, its argument, and `null` and object results. A result that is not an object or `null`, `exec: 1`, no `exec` and a primitive `this` must throw `TypeError`. Then a regexp with its own `exec`, a subclass that overrides it, and a patched `RegExp.prototype.exec`.

Without the fix, three of three runs failed with a `TypeError` on the first assertion. With it, three passed.

## What the maintainer did

@dop251 merged it with no comments.

## The lesson

- A fast path needs a row for each way an object can differ: an own `exec` fails a guard that accepts any regexp object, a subclass one that reads a single flag.
- Test where the spec branches. A non-callable `exec` falls back to the built-in matcher, so `r.exec = 1; r.test("a")` stays `true`. A helper that throws fails that row.

If an engine of yours special-cases its own regexp type before calling `exec`, reply on [X](https://x.com/spicysharkco).
