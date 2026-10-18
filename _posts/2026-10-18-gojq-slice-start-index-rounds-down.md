---
layout: post
title: "Truncation where jq rounds down: slice start index in gojq"
date: 2026-10-18
project: gojq
pr: https://github.com/itchyny/gojq/pull/322
status: merged
tags: [observability-data, go, jq, rounding]
---

In gojq, a pure Go implementation of jq, a slice with a negative fractional start such as `.[-1.5:]` returned a different result than jq. I fixed it in [PR #322](https://github.com/itchyny/gojq/pull/322), merged about six hours after I opened it. It was my first gojq PR.

## When it happens

The same query in gojq before the fix and in jq 1.8.2:

```sh
$ echo '[0,1,2]' | gojq -c '.[-1.5:], .[-0.5:], (.[-1.5:] = ["x"])'
[2]
[0,1,2]
[0,1,"x"]
$ echo '[0,1,2]' | jq -c '.[-1.5:], .[-0.5:], (.[-1.5:] = ["x"])'
[1,2]
[2]
[0,"x"]
```

Strings differ too: `"abcde" | .[-1.5:]` is `"e"` in gojq and `"de"` in jq.

## How slicing works in gojq

`slice`, `sliceString` and `updateArraySlice` in `func.go` convert the start index with `ToInt`, which cuts the fraction off toward zero. A negative result counts from the end. So -1.5 becomes -1, and -0.5 becomes 0, the start of the array. jq rounds down to -2 and -1.

## Why the gap existed

The README had listed fractional indices as a difference from jq since 2022, with positive examples only. In September 2025 @itchyny changed the end index to round up (`0119d61`) and left the start as it was. Three days later the README line was removed. Every fractional start in the old slice tests is positive, where truncation and rounding down agree.

## The fix

A new `toIntFloor` sits next to `toIntCeil`: `math.Floor`, then `ToInt`. The three start conversions call it. The diff is trimmed: it has no context lines and omits the same one-line change in `sliceString` and `updateArraySlice`.

```diff
@@ -1062 +1062 @@ func slice(vs []any, e, s any) any {
-		if i, ok := ToInt(s); ok {
+		if i, ok := toIntFloor(s); ok {
@@ -2166,0 +2167,7 @@ func toIntCeil(x any) (int, bool) {
+
+func toIntFloor(x any) (int, bool) {
+	if f, ok := x.(float64); ok {
+		x = math.Floor(f)
+	}
+	return ToInt(x)
+}
```

## The test

Two cases in `cli/test.yaml` fail before the fix and pass after it. The array case runs `.[-1.5:]`, `.[-0.5:]`, `.[-2.5:-1.5]`, an assignment and `del(.[-1.5:])`. The string case runs the first three on `"１２３４５"`. Reverting any one of the three call sites fails a case. The package tests passed.

## What the maintainer did

@itchyny approved with "Thanks." and merged it. The merged tree equals my commit's tree.

## The lesson

- Truncation and rounding down agree from zero up, so positive fractions cannot tell them apart. Add a negative fraction, and one between -1 and 0, where truncation gives `0`.
- Give each end of a range its own rounding helper. gojq has `toIntCeil` for the end and `toIntFloor` for the start. The new cases slice `.[-2.5:-1.5]`, which uses both.
- Read the docs history before you call a difference a bug. The README note went three days after the end index changed.

If a slice in your own code converts a float index, reply on [X](https://x.com/spicysharkco).
