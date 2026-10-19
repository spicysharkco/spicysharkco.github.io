---
layout: post
title: "An escaped emoji decoded as two U+FFFD: JSON strings in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/254
status: merged
tags: [observability-data, go, json]
---

The `oj`, `gen` and `sen` parsers in OjG, a Go library, decoded an emoji written as an escaped UTF-16 pair, `\ud83d\ude00`, as two U+FFFD. I fixed it in [PR #254](https://github.com/ohler55/ojg/pull/254), merged about five minutes after I opened it.

## When it happens

```go
v, _ := oj.ParseString(`"\ud83d\ude00"`)
fmt.Printf("%+q\n", v)
// "\ufffd\ufffd"   want "\U0001f600"
```

RFC 8259 section 7 writes a character outside the Basic Multilingual Plane as such a pair, and `encoding/json` decodes this input to U+1F600. Object keys and both tokenizers failed the same way.

## How a string escape is decoded

`parseBuffer` in `oj/parser.go` writes each `\u` rune to the string buffer, `p.tmp`, with `utf8.EncodeRune`. For a surrogate half that writes U+FFFD, three bytes, so a pair gave two.

## Why the gap existed

The parsers encoded each escape alone from 2020, the tokenizers from 2021, and none of the five test files held a surrogate escape.

## The fix

Each decoder now keeps `hi`, its last `\u` rune, and `hiEnd`, the length of `p.tmp` just after it. If the next escape follows with nothing written between and `utf16.DecodeRune` joins the two, the three U+FFFD bytes are cut and the joined rune is written. Both live in the struct, so a pair split by a reader refill still joins. `hi` is cleared when a string closes and when a parse starts.

Trimmed `oj/parser.go` diff, without context lines, hunk labels, import, fields or clears. The other four decoders have the same block.

```diff
@@ -522,0 +529,8 @@
+				// A low surrogate escape right after a high one is the second
+				// half of a pair. Replace the U+FFFD written for the high half.
+				if p.hiEnd == len(p.tmp) {
+					if r := utf16.DecodeRune(p.hi, p.rn); r != utf8.RuneError {
+						p.tmp = p.tmp[:len(p.tmp)-3]
+						p.rn = r
+					}
+				}
@@ -524,0 +539,2 @@
+				p.hi = p.rn
+				p.hiEnd = len(p.tmp)
```

## The test

`TestParserParseString` in `oj/parser_test.go` gained thirteen rows, twelve with a value and an error row. Five fail without the fix: pairs, a pair in a key, high-high-low and high-low-low. Eight pass either way: a half with no partner stays U+FFFD, as in `encoding/json`, and a bad second escape is an error. Two more rows, in `TestParserParseReaderMany`, split a pair at the 4096-byte buffer.

Remove the clear at string close and only `["\ud83d","abc\ude00"]` fails (a mutant run on `oj/parser.go`): `abc` is three bytes, the `hiEnd` the first string left, so the stale half joins `\ude00`.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The merged files match my branch.

## The lesson

- A saved offset means nothing once its buffer holds new text. Clear the state that uses it where old text can end: a string closing, a new parse.
- A test for stale state must land new text on the stale offset, as `abc` does.

If a decoder of yours saves an offset into its buffer, reply on [X](https://x.com/spicysharkco).
