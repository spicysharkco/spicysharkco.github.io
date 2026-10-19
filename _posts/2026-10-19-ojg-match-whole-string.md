---
layout: post
title: "Anchors that split at an alternation: match() in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/255
status: merged
tags: [observability-data, go, jsonpath]
---

In the Go library OjG, a JSONPath `match()` pattern with a top-level `|` was not tested against the whole string: `ab|cd` also matched `abx` and `xcd`, though RFC 9535 section 2.4.6 has `match()` test the entire string. I fixed it in [PR #255](https://github.com/ohler55/ojg/pull/255), merged about four hours after I opened it.

## When it happens

```go
x := jp.MustParseString(`$[?match(@, 'ab|cd')]`)
fmt.Println(x.Get([]any{"ab", "cd", "abx", "xcd"}))
// [ab cd abx xcd]   want [ab cd]
```

## How `match` anchors a pattern

The `match` case in `evalStack` put `^` before the pattern unless it began with one, and `$` after it unless it ended with one. The alternation `|` binds loosest in Go's regexp syntax, so `^ab|cd$` is `(^ab)|(cd$)`: the anchors split, `^` holding only the first alternative and `$` only the last.

## Why the gap existed

The anchoring dates from March 2023. The two `match` rows in `TestScriptEval` used `ab.` and `ab`, with no `|`.

## The fix

The pattern is wrapped as `^(?:` + pattern + `)$`, keeping the `|` inside a group. That replaces the two checks for an anchor already present: `^(?:^ab$)$` matches what `^ab$` does. The `jp/script.go` diff, without context lines or the import:

```diff
@@ -832,8 +833,6 @@ func evalStack(sstack []any) []any {
-					if rs[0] != '^' {
-						rs = "^" + rs
-					}
-					if rs[len(rs)-1] != '$' {
-						rs += "$"
-					}
-					if rx, err := regexp.Compile(rs); err == nil {
-						sstack[i] = rx.MatchString(ls)
+					// Parse the pattern alone first, since a pattern such as
+					// "a)(b" is only valid once wrapped in the group.
+					if _, err := syntax.Parse(rs, syntax.Perl); err == nil {
+						if rx, err := regexp.Compile("^(?:" + rs + ")$"); err == nil {
+							sstack[i] = rx.MatchString(ls)
+						}
```

With the wrap alone, `match(@.x, 'a)(b')` returned true for `"ab"`: `a)(b` is invalid, but `^(?:a)(b)$` compiles. The pattern is now parsed alone first, with the flags `regexp.Compile` uses; an invalid pattern gives no match.

## The test

Twelve rows joined the `TestScriptEval` table: three for `ab|cd`, eight for `^ab`, `ab$`, `^ab$` and `^ab|cd$`, and one for `a)(b`. Three fail without the fix (`abx` and `xcd` for `ab|cd`, `abx` for `^ab|cd$`); the other nine pass either way.

## What the maintainer did

@ohler55 thought patterns with `^` or `$` might break and asked: “Can you add tests for those patterns and see if they still work?” They approved, squash-merged it and wrote “Thanks”. The merged files match my branch.

## The lesson

- Put text you did not write in a group before you add syntax beside it. A one-alternative row cannot see the bug: `ab` against `abc` passed on the base.
- Wrapping can make invalid input valid. Parse it alone first: the `a)(b` row passes on the base and fails with the wrap alone.

If you build a regexp by putting `^` and `$` around caller text, reply on [X](https://x.com/spicysharkco).
