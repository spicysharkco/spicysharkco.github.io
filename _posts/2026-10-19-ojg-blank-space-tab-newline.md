---
layout: post
title: "A tab, line feed or carriage return refused as blank space: JSONPath queries in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/256
status: merged
tags: [observability-data, go, jsonpath]
---

A JSONPath filter written over two lines did not parse in OjG, a Go library for JSON. RFC 9535 section 2.1.1 defines blank space as a space, tab, line feed or carriage return; the parser took only the space. I fixed it in [PR #256](https://github.com/ohler55/ojg/pull/256), merged about two hours and twenty minutes after I opened it.

## When it happens

```go
_, err := jp.ParseString("$[?\n@.a]")
fmt.Printf("%q\n", err)
// "'' is not a value or function at 4 in $[?\n@.a]"
```

It should parse as `$[?(@.a)]`, as `$[? @.a]` already does. Blank space in brackets and around a filter operator failed too: `"abc[\t-2\t].def"` and `"$[?\n@.x\n==\n1\n]"`.

## How blank space is skipped

`skipSpace` and `nextNonSpace` in `jp/parse.go` stepped over spaces and stopped at any other byte. `afterBracket` and `readUnion` tested for `' '` themselves before they called `skipSpace`.

## Why the gap existed

The parser came in June 2020 with all four of those checks, and its blank-space tests used only spaces, as in `abc[ -2 ].def`.

## The fix

`spaceMap` is a 256-byte string constant marking a space, tab, line feed and carriage return. `skipSpace`, `nextNonSpace` and `readUnion` look the byte up in it. `afterBracket` switches on the byte, so its `case` lists the four. A form feed and a blank right after a dot, as in `"$.\ta"`, stay errors.

Trimmed `jp/parse.go` diff, hunk headers cut to `@@`, without context lines, the table's two comment lines or its six all-dot rows. After the table the hunks are `afterBracket`, `readUnion`, `skipSpace` and `nextNonSpace`.

```diff
@@
+	spaceMap = "" +
+		".........ss..s.................." + // 0x00
+		"s..............................." + // 0x20
@@
-		case ' ':
+		case ' ', '\t', '\n', '\r':
@@
-			if b == ' ' {
+			if spaceMap[b] == 's' {
@@
-		if b != ' ' {
+		if spaceMap[b] != 's' {
@@
-		if b2 != ' ' {
+		if spaceMap[b2] != 's' {
```

## The test

`TestParse` gained ten rows. Eight hold blank space: a tab, a line feed and a carriage return in brackets, one union row, one slice row and three filter rows. Two are error rows, `"$.\ta"` and `"$[\f'a']"`. Run one at a time without the fix, the eight fail and the two pass. `go test ./jp -run 'TestParse$'` fails without the fix and passes with it.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it into `develop`. The merged files match my branch.

## The lesson

- Search for the literal, `' '`, and not only the helper. Revert `afterBracket` alone and `abc[\t-2\t].def` fails; revert `readUnion` alone and the union row fails (mutant runs).
- Give each member of a set a row of its own, so a failure names it. Without carriage return in the fix, `abc[\r-2\r].def` is the first row to fail.

If a parser of yours tests for a space alone, reply on [X](https://x.com/spicysharkco).
