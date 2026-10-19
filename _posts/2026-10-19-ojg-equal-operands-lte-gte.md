---
layout: post
title: "Equal operands that never satisfied <= or >=: filter comparisons in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/251
status: merged
tags: [observability-data, go, jsonpath]
---

In OjG, a Go JSON library with JSONPath, `<=` and `>=` in a filter never matched two equal `null`, boolean, array or object values, or two missing values. `==` did. I fixed it in [PR #251](https://github.com/ohler55/ojg/pull/251), merged about ten hours after I opened it.

## When it happens

```go
doc := oj.MustParseString(`[{"a":null},{"a":true},{"a":false}]`)
jp.MustParseString(`$[?@.a<=null]`).Get(doc)
// []   want [{"a":null}]
jp.MustParseString(`$[?@.a>=true]`).Get(doc)
// []   want [{"a":true}]
```

RFC 9535 section 2.3.5.2.2: "The comparison a <= b yields true if and only if a < b yields true or a == b yields true". `>=` is defined the same way with `b < a`.

## How the comparison is evaluated

`evalStack` in `jp/script.go` handles `<=` and `>=`. Each case set the result to `false`, then switched on the type of the left operand. Numbers and strings got a real comparison. Every other pair kept `false`.

## Why the gap existed

The cases date from May 2020. The first version compared numbers and strings and gave `false` for any other type. The `<=` and `>=` rows in `TestScriptEval` used only numbers and strings.

## The fix

The whole `jp/script.go` diff:

```diff
@@ -614,7 +614,8 @@ func evalStack(sstack []any) []any {
 				sstack[i] = ok && tl > tr
 			}
 		case lte.code:
-			sstack[i] = false
+			// Equal operands of any type satisfy <=; numbers and strings can also be less.
+			sstack[i] = scriptEqual(left, right)
 			switch tl := left.(type) {
 			case int64:
 				switch tr := right.(type) {
@@ -635,7 +636,8 @@ func evalStack(sstack []any) []any {
 				sstack[i] = ok && tl <= tr
 			}
 		case gte.code:
-			sstack[i] = false
+			// Equal operands of any type satisfy >=; numbers and strings can also be greater.
+			sstack[i] = scriptEqual(left, right)
 			switch tl := left.(type) {
 			case int64:
 				switch tr := right.(type) {
```

Both cases now start from `scriptEqual`, the function `==` uses. The switch below each line still sets the result for numbers and strings.

## The test

Fifteen rows in `TestScriptEval`: eight for `<=`, seven for `>=`. Ten are match rows and fail without the fix. Five are no-match rows and pass either way. Two of them:

```go
{src: "(@.x <= @.y)", value: map[string]any{"a": 1}},
{src: "(@.x <= null)", value: map[string]any{"a": 1}, noMatch: true},
```

Two missing values match. A missing value against `null` does not.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The merged change is the same as my branch's.

## The lesson

- A default set before a type switch is also the answer for every type the switch leaves out. Here `false` was the answer for `null`, booleans, arrays and objects.
- A fix that adds matches needs rows that must not match. An always-true version passes every match row and fails `(@.a <= false)` with `a` set to `true`.

If a comparison of yours starts from `false` before a type switch, reply on [X](https://x.com/spicysharkco).
