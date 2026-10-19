---
layout: post
title: "An empty pattern that matched nothing: match() and search() in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/257
status: merged
tags: [observability-data, go, jsonpath]
---

An empty pattern, `''`, made `match()` and `search()` select nothing in OjG, a Go library for JSON. The empty string is a valid I-Regexp (RFC 9485), so RFC 9535 says `match(@, '')` selects `""` and nothing else, and `search(@, '')` selects every string. [PR #257](https://github.com/ohler55/ojg/pull/257) fixes both. @ohler55 merged it about one hour and thirteen minutes after I opened it.

## When it happens

```go
data := []any{"", "a"}
fmt.Printf("%q\n", jp.MustParseString(`$[?match(@, '')]`).Get(data))
fmt.Printf("%q\n", jp.MustParseString(`$[?search(@, '')]`).Get(data))
// []
// []
```

These should print `[""]` and `["" "a"]`.

## How the pattern check works

`evalStack` in `jp/script.go` reads the pattern with `rs, _ := right.(string)` and runs the match only when `0 < len(rs)`. The `_` drops the `ok` that says whether the value is a string. So the length test had two jobs: skip a pattern that is not a string, and, as a side effect, skip `''`.

## Why the gap existed

The `match` and `search` cases have had that guard since March 2023. Back then `match` read the first and last byte of the pattern to find anchors, which needs a non-empty pattern. In October 2026 that code was replaced and the guard stayed. `search` never read those bytes. No `match` or `search` row in `TestScriptEval` used an empty pattern.

## The fix

Trimmed `jp/script.go` diff, hunk header cut to `@@`. `match` and `search` each have this pair of lines; one is shown.

```diff
@@
-				if rs, _ := right.(string); 0 < len(rs) {
+				if rs, ok := right.(string); ok {
```

For a non-empty string, `0 < len(rs)` and `ok` are both true. For a pattern that is not a string, or is missing, both are false, so it still gives no match. Only `''` changes. `match` wraps it as `^(?:)$`, which matches only `""`. `search` compiles an empty regexp, which matches every string.

## The test

Six rows in `TestScriptEval`. Without the fix three fail: `match` of `""`, and `search` of `""` and of `"abc"`. Three pass either way: `match` of `"a"`, a number pattern against `""`, and a missing `search` pattern against `"abc"`. `go test ./jp -run 'TestScriptEval$' -count=3 -v` failed 3 of 3 before the fix and passed 3 of 3 after.

## What the maintainer did

@ohler55 approved it and squash-merged it into `develop`. The merged files match my branch.

## The lesson

- A length test on a value from a type assertion does two jobs. `rs, _ := x.(string); 0 < len(rs)` asks "is it a string" and "is it non-empty" at once. Test `ok` for the first.
- Give the wrong-type case a row an empty pattern would match. With the guard made always true, `match(@.x, 5)` against `""` fails. The same row against `"a"` would not.
- When the code a guard protected is replaced, read the guard again.

If a function of yours guards a type assertion with `len`, reply on [X](https://x.com/spicysharkco).
