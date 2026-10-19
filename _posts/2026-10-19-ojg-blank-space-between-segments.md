---
layout: post
title: "Blank space refused before a segment and in a slice: JSONPath parsing in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/258
status: merged
tags: [observability-data, go, jsonpath]
---

`$ .a` and `$[1:5 :2]` were parse errors in OjG, a Go library for JSON, though RFC 9535 allows blank space before a path segment and between the parts of a slice. [PR #258](https://github.com/ohler55/ojg/pull/258) accepts both. @ohler55 merged it about forty minutes after I opened it.

## When it happens

```go
_, err := jp.ParseString("$ .a")
fmt.Printf("%q\n", err)
// "parse error at 2 in $ .a"
```

`$[1:5 :2]` failed with `invalid slice syntax at 7`.

## How the parser reads a path

`nextFrag` in `jp/parse.go` reads the next segment. A blank falls to its `default` case, which ends the expression. `readSlice` took the byte after the end value and after the second colon as it stood. After the first colon it skipped blanks, then read a number whatever came next, so `$[1: ]` tried to read a number at `]`.

## Why the gap existed

The `default` case and `readSlice` date from June 2020, and `readSlice` read the bytes after the end value and the second colon raw in its first version. `TestParse` had blank space only inside brackets and around filter operators. Its one slice row, `$[1\n:\t5]`, put it around the first colon.

## The fix

Trimmed `jp/parse.go` diff: hunk headers cut to `@@`, no context lines, the second-colon change (two sites) shown once.

```diff
@@
+		if !first && !lastDescent && spaceMap[p.buf[p.pos]] == 's' {
+			// Blank space can come before a segment but not after the last one.
+			start := p.pos
+			if b := p.nextNonSpace(); b != '.' && b != '[' {
+				p.pos = start
+			}
+		}
@@
-		b = p.buf[p.pos]
-		p.pos++
+		b = p.skipSpace()
@@
-	} else {
+	} else if b != ']' {
@@
+		if spaceMap[b] == 's' {
+			b = p.skipSpace()
+		}
```

The last block is added after the end value and, one tab less deep, before the closing `]`. Blank space is taken only when `.` or `[` follows; otherwise `nextFrag` puts `p.pos` back.

## The test

Twenty-one rows in `TestParse`. Twelve must parse: six in slices, three between segments, three in filters and function arguments. Nine must stay errors, among them `$ *`. Eight give the same error as before. `$[1:5 3]` now reports position 8 where it said 7.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it into `develop`. The merged files match my branch.

## The lesson

- Test an optional part when it is absent and blank space follows. `$[1: ]` has no end value, but the check for one ran before the blanks were skipped.
- A lookahead that skips must put the position back when it declines. Without `p.pos = start`, `$ *` parses as `$.*` and `$.a b` fails at 5, not 4.

If a parser of yours skips blank space before it knows what follows, reply on [X](https://x.com/spicysharkco).
