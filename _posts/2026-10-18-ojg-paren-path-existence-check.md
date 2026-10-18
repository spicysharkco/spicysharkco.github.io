---
layout: post
title: "A group that dropped the existence check: parenthesized paths in OjG"
date: 2026-10-18
project: OjG
pr: https://github.com/ohler55/ojg/pull/246
status: merged
tags: [observability-data, go, jsonpath]
---

In OjG, a Go JSON library with JSONPath, a bare `@.x` in a filter tests that `x` exists, but `(@.x)` beside `&&`, `||` or `!` did not. I fixed it in [PR #246](https://github.com/ohler55/ojg/pull/246), merged about 37 minutes after I opened it.

## When it happens

```go
doc := oj.MustParseString(`[{"a":1},{"b":2},{"a":1,"b":2}]`)
jp.MustParseString(`$[?(@.a) && (@.b)]`).Get(doc)
// []                   want [{"a":1,"b":2}]
jp.MustParseString(`$[?!(@.a)]`).Get(doc)
// all three elements   want [{"b":2}]
```

The bare forms `$[?@.a && @.b]` and `$[?!@.a]` were right. RFC 9535 lists grouping `(...)` in its precedence table (section 2.3.5.1) and says a query by itself in a logical context is an existence test (section 2.3.5.2.1). The same cause drops `{"a":1}` from `$[?(@.a) || (@.b && @.c)]`, the case noted at the end of [PR #241](https://github.com/ohler55/ojg/pull/241).

## How a filter script works

For each element, `evalWithRoot` copies the filter into a stack. A path that finds a value becomes a `got`; one that finds nothing becomes `Nothing`. `evalStack` unwraps each `got` and notes it in `gleft` or `gright`. `&&`, `||` and `!` count a `got` as found, whatever its value.

## Why the gap existed

"Group eval fix (#162)" added the group case in February 2024, before any `got` existed. "Filter ancestors (#209)", in June 2025, added the `got` ("Bare @.x in filter should return existence") and made the three operators read it. The group case kept copying the bare value. Every test with a parenthesized path made it the whole filter, as in `$[?(@.x)]`; none put one beside `&&`, `||` or `!`.

## The fix

When the grouped value came from a path, the group stores a `got` again. A missing path is `Nothing`, so it still counts as not found. The whole `jp/script.go` diff:

```diff
@@ -562,6 +562,10 @@ func evalStack(sstack []any) []any {
 		switch o.code {
 		case group.code:
 			sstack[i] = left
+			if gleft {
+				// Keep the got so (@.x) stays an existence check, as @.x is.
+				sstack[i] = &got{value: left}
+			}
 		case eq.code:
 			sstack[i] = scriptEqual(left, right)
 		case neq.code:
```

Under `&&`, `||` or `!`, `(@.x)` now counts as found when `x` is `false`, `null`, `0`, `""` or an empty array, as `@.x` does.

## The test

Five rows joined `TestScriptEval`: `((@.x) && (@.y))`, `((@.x) || (@.y && @.z))`, and `(!(@.x))` with `x` set to 1, to `false` and missing. Four fail without the fix. The missing-path row guards against counting a missing path as found.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The diff did not change.

## The lesson

- A wrapper that carries meaning must survive every case that passes a value on.
- Test a construct beside each operator that reads it. A parenthesized path that is the whole filter never meets `&&`, `||` or `!`.
- Existence is not truthiness. A row with `x` set to `false` tells them apart.

If a filter in your own code unwraps a value inside a group, reply on [X](https://x.com/spicysharkco).
