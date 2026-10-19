---
layout: post
title: "A dot that matched a carriage return: match() and search() in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/259
status: merged
tags: [observability-data, go, jsonpath]
---

`match(@, '.')` and `search(@, '.')` selected a string that was one carriage return in OjG, a Go JSON library. RFC 9535 sections 2.4.6 and 2.4.7 take an I-Regexp, and RFC 9485 section 5.3 says: "For any unescaped dots (.) outside character classes (first alternative of charClass production), replace the dot with [^\n\r]." Go's dot leaves out only the line feed. [PR #259](https://github.com/ohler55/ojg/pull/259) closes the gap; @ohler55 merged it about two hours after I opened it.

## When it happens

With `fmt` and `jp` imported:

```go
data := []any{"\r", "a"}
fmt.Printf("%q\n", jp.MustParseString(`$[?match(@, '.')]`).Get(data))
fmt.Printf("%q\n", jp.MustParseString(`$[?search(@, '.')]`).Get(data))
// ["\r" "a"]
// ["\r" "a"]
```

Both should print `["a"]`.

## How the dot is rewritten

`dotNotCR(pattern, value)` runs in `evalStack` in `jp/script.go`, before `match` and `search` compile the pattern. A value without a carriage return keeps the pattern as written; so does a pattern holding `(?`, `\Q`, `[:`, `[]` or `[^]`, forms the scan does not follow. Otherwise each dot outside a bracket class becomes `[^\n\r]`, and a backslash is copied with the next byte, so `\.` stays a literal dot.

## Why the gap existed

`match` and `search` have compiled the pattern with Go's `regexp` since March 2023, and no row in `TestScriptEval` had a carriage return.

## The fix

The scan as merged, without the signature, doc comment and early returns:

```go
buf := make([]byte, 0, len(pattern)+16)
class := false
for i := 0; i < len(pattern); i++ {
	switch c := pattern[i]; {
	case c == '.' && !class:
		buf = append(buf, `[^\n\r]`...)
		continue
	case c == '\\' && i+1 < len(pattern):
		buf = append(buf, c)
		i++
	case c == '[' || c == ']':
		class = c == '['
	}
	buf = append(buf, pattern[i])
}
return string(buf)
```

## The test

Nineteen rows join `TestScriptEval`, thirteen for `match()` and six for `search()`. Seven fail without the fix, counted one row at a time; each has a bare dot that meets a carriage return. The other twelve pass either way, among them an escaped dot and a dot in a class.

## What the maintainer did

@ohler55 left one review comment on the `strings.Builder` and its function calls: “why not use a simple `[]byte`?” The builder took one call per byte. I changed the function to append to one `[]byte` and convert it once at the end. The test file did not change. @ohler55 replied “Looks good.” and approved, then squash-merged it into `develop`.

## The lesson

- When a reviewer asks for a cheaper build, make that change and leave the rows alone. The same nineteen rows were the check.
- A near miss needs its own row. `a.\r` against `"a\n\r"` fails a rewrite to `[^\r]` alone (mutant run).
- Test a lookahead at the end of the input. `.\` is invalid either way, yet without the bound `i+1 < len(pattern)` the scan panics (mutant run).

If you rewrite regexp text before you compile it, reply on [X](https://x.com/spicysharkco).
