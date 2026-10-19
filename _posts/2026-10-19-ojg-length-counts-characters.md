---
layout: post
title: "Bytes counted instead of characters: length() of a string in OjG"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/250
status: merged
tags: [observability-data, go, jsonpath]
---

`length()` of a string in OjG, a Go JSON library with JSONPath, returned the number of bytes, so `"жж"` had a length of 4. I fixed it in [PR #250](https://github.com/ohler55/ojg/pull/250), merged about ten hours after I opened it.

## When it happens

```go
doc := oj.MustParseString(`["a","ab","жж","☺☺","😀"]`)
jp.MustParseString(`$[?length(@)==2]`).Get(doc)
// ["ab"]   want ["ab","жж","☺☺"]
jp.MustParseString(`$[?length(@)==1]`).Get(doc)
// ["a"]    want ["a","😀"]
```

RFC 9535 section 2.4.4 defines the length of a string as its number of Unicode scalar values.

## How length reads a string

`evalStack` in `jp/script.go` switches on the type of the operand of `length`. The string case used `len(tl)`, which in Go counts bytes: `a` is 1, `ж` is 2, `☺` is 3 and `😀` is 4. So `"жж"` measured 4, `"☺☺"` 6 and `"😀"` 4.

## Why the gap existed

`length` arrived in [#119](https://github.com/ohler55/ojg/pull/119) in March 2023 with `len(tl)` in the string case. The only string row among the `length` tests in `jp/script_test.go` was `"abc"`, where bytes and characters agree.

## The fix

The whole `jp/script.go` diff:

```diff
@@ -8,6 +8,7 @@ import (
 	"regexp"
 	"strconv"
 	"strings"
+	"unicode/utf8"
 
 	"github.com/ohler55/ojg"
 	"github.com/ohler55/ojg/gen"
@@ -811,7 +812,7 @@ func evalStack(sstack []any) []any {
 			sstack[i] = Nothing
 			switch tl := left.(type) {
 			case string:
-				sstack[i] = int64(len(tl))
+				sstack[i] = int64(utf8.RuneCountInString(tl))
 			case []any:
 				sstack[i] = int64(len(tl))
 			case map[string]any:
```

Arrays and objects still use `len`.

## The test

One row in `TestScriptEval`: `(length(@.x) == 4)` with `"x": "aж☺😀"`, one character of each width. That string is 10 long in bytes, 5 in UTF-16 units and 4 in characters. Without the fix the length is 10 and the row fails.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The merged files match my branch.

## The lesson

- Put one character of each width in a test string. `"жж"` is 2 as characters and as UTF-16 units; `"aж☺😀"` is 4, 5 and 10, so neither a byte count nor a UTF-16 count passes it.
- Say so when a fix turns a constant-time call into a linear one. `len` of a string reads nothing; `utf8.RuneCountInString` reads all of it. The RFC defines the length that way, so the cost comes with the definition.

If a length function of yours counts bytes, reply on [X](https://x.com/spicysharkco).
