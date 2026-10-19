---
layout: post
title: "A length nothing checked: Array.from on array-likes in goja"
date: 2026-10-19
project: goja
pr: https://github.com/dop251/goja/pull/803
status: merged
tags: [observability-data, go, testing]
---

In goja, a JavaScript engine in Go, `Array.from({length: 2**32})` did not return in any practical time. Node throws `RangeError: Invalid array length`. I fixed it in [PR #803](https://github.com/dop251/goja/pull/803), merged about 34 hours after I opened it.

## When it happens

```js
Array.from({length: 2**32});  // never returns, want RangeError
```

A map function changes nothing. It needs `this` to be `Array` or not a constructor.

## How Array.from works

The spec (ECMA-262 23.1.2.1) builds the result with the length: `Construct(ctor, « 𝔽(length) »)`, or `ArrayCreate(length)` when `this` is not a constructor. `Array(2**32)` and `ArrayCreate` throw a RangeError.

In `array_from`, `l` holds the length. With no constructor to call, goja made the result with `r.newArrayValues(nil)`, an empty array that never saw `l`, then looped `for k := int64(0); k < l; k++`, reading each index and adding one element with `createDataPropertyOrThrow`. A Proxy that counts index reads shows the loop running one index at a time.

## Why the gap existed

The spec's RangeError is a side effect of creating the array with a length. goja's no-constructor branch creates it without one, so the side effect was lost. `TestArrayFrom` used sources of three elements.

## The fix

The whole diff to `builtin_array.go`:

```diff
@@ -1515,6 +1515,9 @@ func (r *Runtime) array_from(call FunctionCall) Value {
 		if ctor != nil {
 			arr = ctor([]Value{intToValue(l)}, nil)
 		} else {
+			if l > math.MaxUint32 {
+				panic(r.newError(r.getRangeError(), "Invalid array length"))
+			}
 			arr = r.newArrayValues(nil)
 		}
 		if mapFn == nil {
```

It throws the same RangeError as `builtin_newArray`. The constructor branch is untouched, and `2**32-1` is still accepted, as in Node.

## The test

`TestArrayFromArrayLikeMaxLength` uses `length: 2**32` and a getter on index 0 that throws a `TestError`. `Array.from(src)`, the same with a map function, and `Array.from.call({}, src)` must each throw RangeError. Then `src.length = 2**32 - 1` must reach the getter and throw `TestError`.

Without the fix, three of three runs failed at the first assertion with `Expected a RangeError but got a TestError`. With it, three of three passed. Changing `>` to `>=` fails the last one.

## What the maintainer did

@dop251 commented on the test: "I think this can be improved by defining a custom Error class and throwing it instead of generic Error which can be anything." I pushed a commit: the test declares `class TestError extends Error {}`, the getter throws it and the last assertion expects it. About 17 hours after my reply, @dop251 squash-merged it, and both files match my last commit.

## The lesson

- A Proxy that counts index reads shows a loop running one index at a time.
- Put a throwing getter on the first element: without the fix the call fails at the first read instead of hanging.
- Throw a class of your own from a test probe. `assert.throws` here compares the constructor exactly, so `Error` accepts any plain `Error`.

If an engine of yours builds a result without checking the length, reply on [X](https://x.com/spicysharkco).
