---
layout: post
title: "Go's == where value equality belongs: the in filter in OjG"
date: 2026-10-18
project: OjG
pr: https://github.com/ohler55/ojg/pull/243
status: merged
tags: [observability-data, go, jsonpath]
---

The `in` filter in OjG, a Go JSON library with JSONPath, panicked when its left operand was an array or object and the list held a member of the same kind. I fixed it in [PR #243](https://github.com/ohler55/ojg/pull/243), merged about twelve and a half hours after I opened it.

## When it happens

```go
import (
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
)

data := oj.MustParseString(`[{"a":[1],"b":[[1]]}]`)
jp.MustParseString("$[?@.a in @.b]").Get(data)
```

This panicked with `comparing uncomparable type []interface {}`. It should return the row: `[1]` is in `[[1]]`. An object, as in `{"a":{"x":1},"b":[{"x":1}]}`, panicked the same way, and so did `[1]` against `[[2]]`.

## How `in` works

In `jp/script.go`, the `in` case of `evalStack` loops over the right operand, a list, and answers true at the first member where `left == ev`. Go's `==` on two interface values panics when both hold the same uncomparable type, such as two slices. For different types it returns false, so `[1] in [1,2]` never crashed.

## Why the gap existed

The loop came with the operator in April 2022. [PR #242](https://github.com/ohler55/ojg/pull/242), merged on 12 October, added `scriptEqual`, which compares arrays and objects element by element with `int64` and `float64` equal by value. It replaced the raw `==` in the `==` and `!=` operators and did not touch `in`, a separate branch. The existing `in` rows held only integers and strings.

## The fix

The loop calls `scriptEqual(left, ev)`, so `in` agrees with `==` and `!=`. The `jp/script.go` diff is trimmed: no context lines. The test rows are described below.

```diff
@@ -769 +769 @@ func evalStack(sstack []any) []any {
-					if left == ev {
+					if scriptEqual(left, ev) {
```

Two answers that did not panic change too: `(@ in [1])` was false for `1.0`, and `(@ in [1.0])` was false for `1`. Both are now true, as `(@ == 1)` and `(@ == 1.0)` already were.

## The test

Six rows joined the `TestScriptEval` table. Four use `(@.a in @.b)`: a matching and an unequal member, for arrays and for objects. The last two are `(@ in [1])` with `1.0` and `(@ in [1.0])` with `1`. Before the fix the first four panic and the last two return false; after it all six pass. Put the old `left == ev` back and the test panics again.

## What the maintainer did

@ohler55 squash-merged it with no review and no change to the diff, and commented “Thanks for the fix.” It was my first OjG PR.

## The lesson

- Go's `==` on interface values panics only when both hold the same uncomparable type. A test needs a list member of the same kind as the left value.
- A new comparator changes answers that never crashed. `scriptEqual` converts in separate branches for an integer and a float on the left, so test both orders.

If your filter language has an `in` that predates its equality helper, reply on [X](https://x.com/spicysharkco).
