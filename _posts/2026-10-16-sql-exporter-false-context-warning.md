---
layout: post
title: "TrimPrefix is not TrimLeft: a false warning on every scrape error in sql_exporter"
date: 2026-10-16
project: sql_exporter
pr: https://github.com/burningalchemist/sql_exporter/pull/1115
status: merged
tags: [observability-data, go, logging]
---

In single target mode, sql_exporter warned about its own log context on every scrape error. I fixed it with a one-word change ([PR #1115](https://github.com/burningalchemist/sql_exporter/pull/1115), merged 2026-10-16 as [72c7582](https://github.com/burningalchemist/sql_exporter/commit/72c7582aa2f5bd113999df06da1067d807667f89)).

## What broke

Each scrape error logged this line:

```
level=WARN msg="Invalid context log item, ignoring" item=""
```

It points at the exporter's own logging, not at your database, so during an incident it is noise you cannot act on. The `scrape_errors_total` labels were right; only the logs were wrong.

An error carries a log context, a comma-separated list like `collector=c,query=q` that says where it came from. In single target mode there is no job or target name, so the outermost parent is empty. Each builder (`%s,collector=%s` and three more) adds `,key=value` to its parent, so the result starts with a comma. `TrimMissingCtx` strips it.

[PR #915](https://github.com/burningalchemist/sql_exporter/pull/915), a four-file job-filter change, replaced a `HasPrefix(",")` check plus `TrimLeft(", ")` with `TrimPrefix(logContext, ", ")`. That strips a leading `", "` and nothing else, and no builder writes a space. So `,collector=c,query=q` kept its comma, and `parseContextLog` saw an empty first item and warned. The same PR had added that warning.

## The fix

```diff
-	return strings.TrimPrefix(logContext, ", ")
+	return strings.TrimLeft(logContext, ", ")
```

`TrimPrefix` removes one exact string. `TrimLeft` removes any run of leading characters from a set. The function's own comment says "Leading comma appears when previous parameter is undefined", so a bare leading comma is the case it exists for.

No test covered the helper before. The PR adds a table test, and a test that builds a collector error with an empty parent context and checks that `parseContextLog` logs nothing. Before the fix, the table test fails 2 of 5 cases and the error test fails on the context and on the log.

## What the maintainer did

@burningalchemist commented about six minutes after I opened the PR. They thanked me and wrote "I guess I missed that back in the day or didn't pay attention to." They approved with no review text and merged 34 seconds later. There were no inline comments or requested changes.

The regression shipped in 0.20.0 (2026-03-12). The fix is on master; the latest release when I checked, 0.24.10, does not include it.

## The lesson

A cleanup that swaps one string function for a simpler-looking one is easy to wave through, because both read as "trim". If you build a string with one separator and strip it with another, write a test with the literal input and output: `",collector=c"` becomes `"collector=c"`.

Test through the consumer too. `parseContextLog` returned the right map and only its log line was wrong, so a test of the return value would pass. Capture the log output and assert it is empty.

If you add a warning for malformed input, push one ordinary failure through it. A warning that fires on every ordinary error means the code that builds the input is wrong.

If a warning of yours turned out to fire on ordinary input, reply on [X](https://x.com/spicysharkco).
