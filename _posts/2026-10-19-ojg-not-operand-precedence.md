---
layout: post
title: "A not that read to the end of the filter: the `!` operator in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/248
status: merged
tags: [observability-data, go, jsonpath]
---

In OjG, a Go JSON library with JSONPath, `!` in a filter took in every `&&` and `||` term after it: `!@.a && @.b` meant `!(@.a && @.b)`. `(!@.a) && @.b` already worked. I fixed the bare form in [PR #248](https://github.com/ohler55/ojg/pull/248), merged about 49 minutes after I opened it.

## When it happens

```go
doc := oj.MustParseString(`[{"a":1},{"b":2},{"a":1,"b":2},{"c":3}]`)
jp.MustParseString(`$[?!@.a && @.b]`).Get(doc)
// [{"a":1},{"b":2},{"c":3}]   want [{"b":2}]
jp.MustParseString(`$[?!@.a || @.b]`).Get(doc)
// [{"c":3}]                   want [{"b":2},{"a":1,"b":2},{"c":3}]
```

RFC 9535 ranks logical NOT above `&&` and `||` (section 2.3.5.1, Table 10), so `!` applies to one term.

## How the parser reads `!`

`readEq` in `jp/parse.go` reads the operand of `!` with `p.readEq()`, a whole equation, so the rest of the filter lands under the `not`. `precedentCorrect` orders operators by precedence (lower binds tighter: `==` 3, `&&` 4, `||` 5) but never splits a chain out from under a `not`.

## Why the gap existed

The parser rework in February 2024 ([#160](https://github.com/ohler55/ojg/pull/160)) gave `!` an operand read by `readEq` and added `precedentCorrect`; before it, `$[?(!@.a && @.b)]` failed with `not terminated`. In `jp/script_test.go`, no row put `&&` or `||` after the term of a `!`.

## The fix

When the corrected operand of a `not` is an `&&` or `||`, the `not` moves onto that node's left term; recursion handles longer chains. The `jp/equation.go` diff, without context lines:

```diff
@@ -377,0 +378,8 @@ func precedentCorrect(e *Equation) *Equation {
+	// The operand of a not is read up to the end of the expression. A not
+	// applies only to the term before the first && or || that follows it.
+	if e.o.code == not.code && e.left.o != nil && and.prec <= e.left.o.prec {
+		l := e.left
+		e.left = l.left
+		l.left = precedentCorrect(e)
+		return l
+	}
```

A `!` before a comparison is outside the RFC grammar and still takes the whole comparison, but what follows it changes: `!@.a == 1 && @.b` was `!(@.a == 1 && @.b)` and is now `!(@.a == 1) && @.b`.

## The test

Six rows joined existing tables, one in `TestScriptParse` and five in `TestScriptEval`. Without the fix both tests fail on their first new row. `!@.x == 1 && @.y` gets two: with `{"x":2,"y":3}` it matches before and after; with `{"x":2}` it matched before and no longer does, because `@.y` is outside the `!`.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The diff did not change.

## The lesson

- An operand read by the general expression reader takes the rest of the input. Read one term, or hand the tail back.
- Where a fix follows a standard, mark where it stops, and say what still changes there.
- A row that claims a case is unchanged proves nothing if old and new code agree on its data: with `y` present both readings match.

If a parser of yours reads operands this way, reply on [X](https://x.com/spicysharkco).
