---
layout: post
title: "A minus sign read as part of the operator: negative numbers in OjG filters"
date: 2026-10-19
project: OjG
pr: https://github.com/ohler55/ojg/pull/252
status: merged
tags: [observability-data, go, jsonpath]
---

`jp.ParseString` in OjG, a Go JSON library with JSONPath, rejected a filter with no space between an operator and a negative number, such as `$[?@.a==-1]`. I fixed it in [PR #252](https://github.com/ohler55/ojg/pull/252), merged about 22 minutes after I opened it.

## When it happens

```go
_, err := jp.ParseString(`$[?@.a==-1]`)
// '==-' is not a valid operation at 9 in $[?@.a==-1]
```

It should parse as `$[?(@.a == -1)]`. RFC 9535 section 2.3.5.1 makes the blank space around a comparison operator optional and lets a number start with `-`. The other comparison operators failed the same way, as did `+`, `-`, `*` and `/` (`@.x+-1`, `@.x--1`, `2*-1`, `@.x/-1`). The spaced form already parsed.

## How the operator is read

`readEqOp` in `jp/parse.go` reads a run of operator characters, and raises when a `-` lands in a token longer than one character. So `==-` is read as one token and rejected.

## Why the gap existed

The check dates from the first version of `readEqOp`, in June 2020. At the base commit no test in `jp` had an operator directly before a minus sign and a digit.

## The fix

The run now stops at a `-` that follows a complete operator and is followed by a digit. The whole `jp/parse.go` diff:

```diff
@@ -702,6 +702,9 @@ func (p *parser) readEqOp() (o *op) {
 			if len(token) == 0 || !partialOp(token, b) {
 				break
 			}
+		} else if b == '-' && opMap[string(token)] != nil && p.pos+1 < len(p.buf) && '0' <= p.buf[p.pos+1] && p.buf[p.pos+1] <= '9' {
+			// A minus directly after an operator and before a digit starts a negative number.
+			break
 		}
 		token = append(token, b)
 		if b == '-' && 1 < len(token) {
```

The token is then `==` and `-1` is the next operand. The `len(p.buf)` test keeps a trailing `==-` an error. `-` is itself a complete operator, so `@.x--1` now parses as `@.x - -1`; it failed before.

## The test

Fifteen rows joined `TestParse`: eleven that must parse and four that must stay errors (`==- 1`, `==-.1`, a trailing `==-`, `=-1`). Two:

```go
{src: "$[?@.x==-1]", expect: "$[?(@.x == -1)]"},
{src: "[?(@.x==-.1)]", err: "'==-' is not a valid operation at 9 in [?(@.x==-.1)]"},
```

Without the fix `TestParse` stops at the first new row; the error rows pass either way. The `*` row is `2*-1` because `@.a*-1` already parsed: `@.a*` reads as the path `@.a.*`.

## What the maintainer did

@ohler55 approved it with no comment and squash-merged it. The merged files match my branch.

## The lesson

- Write the lookahead from the grammar and no wider: a digit, not just "not a space". Give each near miss an error row.
- Run each sibling on the unfixed code before it gets a row. A form can already work for another reason.

If a lexer of yours reads a run of operator characters, reply on [X](https://x.com/spicysharkco).
