---
layout: post
title: "A one-value filter failed: VALUE_IN on list requests in Flyte"
date: 2026-10-19
project: Flyte
pr: https://github.com/flyteorg/flyte/pull/8194
status: merged
tags: [observability-data, go, testing]
---

A Flyte `ListActions` request with a `VALUE_IN` filter and one value failed with an internal error. With two values it returned rows. Flyte orchestrates ML pipelines. [PR #8194](https://github.com/flyteorg/flyte/pull/8194) fixed it and was merged about fourteen hours after I opened it.

## When it happens

`ListActions` with the filter `field: "phase"`, `function: VALUE_IN`, `values: ["6"]` failed, and the error ended in `pq: unable to convert string to array`. In the code, `ListRuns` shares the filter function and repository call. The proto says `VALUE_IN` values "may contain multiple entries", so one value is valid input.

## How a VALUE_IN filter becomes SQL

`ConvertProtoFilters` turns each proto filter into a `basicFilter` with a field, an expression and a value. For `VALUE_IN`, `QueryExpression` writes `column = ANY(?)` and wraps the value in `pq.Array`. `pq.Array` builds a Postgres array from a `[]string`. For a plain string it returns a generic wrapper whose `Value` fails with the error above.

## Why the gap existed

The `VALUE_IN` query and the code that picks the value both date from the day the file was added, in April 2026. With exactly one value, that code passed a plain string on for every function. The branch for more than one value carried the comment "For VALUE_IN, pass the array", and a one-value `VALUE_IN` never reached it.

## The fix

The diff of `filters.go`, trimmed to the changed lines and the line between them:

```diff
@@ -373,4 +373,4 @@
-		if len(protoFilter.Values) == 1 {
+		if len(protoFilter.Values) == 1 && expression != interfaces.FilterExpressionValueIn {
 			value = protoFilter.Values[0]
-		} else if len(protoFilter.Values) > 1 {
-			// For VALUE_IN, pass the array
+		} else if len(protoFilter.Values) >= 1 {
+			// Several values, or one value for VALUE_IN, which binds an array
```

Both edits are needed: with only the first, one value would reach the `has no values` error. Other functions, and zero values, behave as before.

## The test

`TestConvertProtoFilters_ValueInSingleValue` converts one `phase` `VALUE_IN` filter holding "3", builds its expression, and checks the query and the arguments. The query, `phase = ANY(?)`, was right before the fix. The arguments were not: the test expects `pq.Array([]string{"3"})` and without the fix gets `pq.GenericArray{A:"3"}`. With the fix it passes.

## What the maintainer did

@Sovietaced asked "how were you hitting this?" I answered that I found it by reading `ConvertProtoFilters` and then checked it against Postgres. They squash-merged it about three hours later with my diff unchanged.

## The lesson

- Assert the argument, not only the SQL. The query text was the same before and after the fix, so a test on it alone passes without the fix.
- An `interface{}` field moves a type error from build to run time. `basicFilter.value` is one: `ConvertProtoFilters` stores a string or a `[]string` in it, and `pq.Array` takes `any`, so the string compiled and failed only when the driver encoded it.

If a filter layer of yours unwraps one-item lists, reply on [X](https://x.com/spicysharkco).
