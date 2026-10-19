---
layout: post
title: "Escapes refused or read in halves: quoted strings in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/253
status: merged
tags: [observability-data, go, jsonpath]
---

OjG, a Go library for JSON, refused `\/` in a quoted name and read an escaped UTF-16 pair as two U+FFFD. RFC 9535 section 2.3.1.1 allows both. I fixed it in [PR #253](https://github.com/ohler55/ojg/pull/253), merged about five minutes after I opened it.

## When it happens

```go
jp.ParseString(`$["\/"]`)
// 0x2f (/) is not a valid escaped character

jp.MustParseString(`$["\ud83d\ude00"]`).Get(map[string]any{"😀": 1})
// []   want [1]
```

The first should parse as `$['/']`. A high half and a low half, each escaped, are one character, so the second should return `[1]`. Single-quoted names and filter strings act alike.

## How a quoted string is decoded

`readEscStr` in `jp/parse.go` decodes quoted names and strings. Its switch on the character after a backslash had no `/` case, so `\/` hit the error. Its `u` case turns four hex digits into one rune, so a pair gave two runes, each U+FFFD.

## Why the gap existed

`readEscStr` was added in March 2023 with a `/` case. In February 2024 the regexp reader stopped calling `readStr` and scanned for its own closing `/`; the same diff removed the `/` case. No test in `jp` used `\/` outside a regexp or held a surrogate escape.

## The fix

The `jp/parse.go` diff, trimmed: no context lines and no `unicode/utf16` import.

```diff
@@ -457,0 +459,2 @@ top:
+			case '/':
+				buf = append(buf, '/')
@@ -481,0 +485,11 @@ top:
+				// A high surrogate escape followed by a low surrogate escape is one rune.
+				if utf16.IsSurrogate(r) && p.pos+6 <= len(p.buf) && p.buf[p.pos] == '\\' && (p.buf[p.pos+1] == 'u' || p.buf[p.pos+1] == 'U') {
+					var r2 rune
+					for _, c := range p.buf[p.pos+2 : p.pos+6] {
+						r2 = (r2 << 4) | rune(p.readHex(c))
+					}
+					if d := utf16.DecodeRune(r, r2); d != utf8.RuneError {
+						r = d
+						p.pos += 6
+					}
+				}
```

When a surrogate escape is followed by a `\u` or `\U` escape and `utf16.DecodeRune` joins them, the one rune is appended and the second escape skipped. Otherwise the second escape is read on its own, as before.

## The test

Seventeen rows: thirteen in `getTestData` (run by `TestExprGet`) and four in `TestParse`. Without the fix both tests fail: eleven rows fail, six pass either way. Two rows:

```go
{path: `$["\ud83d\ude00"]`, expect: []any{3}, data: map[string]any{"😀": 3, "\ufffd\ufffd": 4}},
{path: `$["\ude00\ud83d"]`, expect: []any{3}, data: map[string]any{"\ufffd\ufffd": 3}},
```

The first holds a decoy key, so the old code returns 4, not nothing. The second passes either way.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The merged files match my branch.

## The lesson

- A case in a shared function serves every caller. Before deleting one, check the others and give it a row.
- Every other pair row has the high half first, so only the swapped row pins the order the grammar sets.

If your decoder reads UTF-16 escapes one at a time, reply on [X](https://x.com/spicysharkco).
