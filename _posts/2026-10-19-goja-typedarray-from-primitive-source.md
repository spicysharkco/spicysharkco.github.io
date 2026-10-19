---
layout: post
title: "Strings and numbers rejected as sources: TypedArray.from in goja"
date: 2026-10-19
project: goja
pr: https://github.com/dop251/goja/pull/805
status: merged
tags: [observability-data, go, testing]
---

`Uint8Array.from("123")` threw in goja, a JavaScript engine in Go: `TypeError: Value is not an object: 123`. Node returns `Uint8Array(3) [ 1, 2, 3 ]`. I fixed it in [PR #805](https://github.com/dop251/goja/pull/805), merged about 75 minutes after I opened it.

## When it happens

```js
Uint8Array.from("123");                         // TypeError, want [1, 2, 3]
Uint16Array.from("abc", c => c.charCodeAt(0));  // TypeError, want [97, 98, 99]
Uint8Array.from(5);                             // TypeError, want an empty array
```

A string or number source hit it. `null` and `undefined` still throw, as in Node.

## How TypedArray.from works

The spec (ECMA-262 23.2.2.1) calls `GetMethod(source, %Symbol.iterator%)` on the source as given (step 5 in the current draft). `ToObject` comes later (step 8) and only for a source without an iterator. A number has none, so it becomes a wrapper with no `length` and gives an empty result.

## Why the gap existed

In April 2020, `typedArray_from` wrapped the source with `Value.ToObject`, which accepts a primitive. A November 2021 rewrite wrote `r.toObject`, which throws for anything but an object. No Go test file called a typed array's `from`.

## The fix

The whole diff to `builtin_typedarrays.go`, with one line of context:

```diff
@@ -1366,6 +1366,6 @@ func (r *Runtime) typedArray_from(call FunctionCall) Value {
 	}
-	source := r.toObject(call.Argument(0))
-	usingIter := toMethod(source.self.getSym(SymIterator, nil))
+	items := call.Argument(0)
+	usingIter := toMethod(r.getV(items, SymIterator))
 	if usingIter != nil {
-		values := r.iterableToList(source, usingIter)
+		values := r.iterableToList(items, usingIter)
 		ta := r.typedArrayCreate(c, intToValue(int64(len(values))))
@@ -1388,2 +1388,3 @@ func (r *Runtime) typedArray_from(call FunctionCall) Value {
 	}
+	source := items.ToObject(r)
 	length := toIntStrict(toLength(source.self.getStr("length", nil)))
```

`r.getV` reads the iterator through a temporary wrapper and keeps the primitive as the receiver, as `Array.from` does. `ToObject` runs only in the array-like branch.

## The test

`TestTypedArrayFromPrimitive` has five assertions: `"123"`, `"abc"` with a map function, `5` with length 0, `5` with an iterator added to `Number.prototype`, and the `this` that iterator saw. The iterator is strict-mode, so `this` stays a number: `sameValue(receiver, 5)`.

Without the fix, three of three runs failed with `Value is not an object: 123`. With it, three passed.

A wrong fix that wraps the primitive up front passes the first four and fails only the last. It prints `Expected SameValue(«5», «5»)`, because one side is a Number object.

## What the maintainer did

@dop251 merged it with a merge commit and no comments. Both files match my commit.

## The lesson

- A wrong fix can pass every row that checks a result: wrapping up front gives the right values for the string and the number. Record the `this` the callee sees, in a strict-mode function.
- Swapping a conversion helper for a similar one changes which inputs throw: `r.toObject(v)` throws on a primitive, `v.ToObject(r)` wraps it. After a rewrite, try a primitive.

If an engine of yours converts its source before asking for an iterator, reply on [X](https://x.com/spicysharkco).
