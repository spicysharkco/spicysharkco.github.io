---
layout: post
title: "I made sql_exporter stop caching a failed query under min_interval"
date: 2026-10-17
---

With `min_interval` set, one failed query in sql_exporter kept returning the same error until the interval ended. I fixed it in [PR #1121](https://github.com/burningalchemist/sql_exporter/pull/1121), merged within twenty minutes of opening it. The cache stored the error like a result.

## When it happens

A collector, or the global setting, has `min_interval: 1h`. A query fails once, for example on a missing table or a lock timeout. A minute later the cause is gone.

The metric stays missing for the hour. Every scrape returns the same error, `scrape_errors_total` rises on each one, and the query is not run again.

I checked it with a CSV table created after the second scrape. Before the fix, scrapes 3 and 4 still failed and `scrape_errors_total` went 1, 2, 3, 4. After the fix they returned `m 1` and the counter stayed at 2.

## How the cache refills

With a non-zero `min_interval`, `cachingCollector.Collect` in `collector.go` runs the queries only when the cache is older than `min_interval` or empty. A loop there sends each metric on and appends it to the cache. Otherwise it replays the cache.

The loop has a comment: `catch invalid metrics and return them immediately, don't cache them`. The condition under it is `ctx.Err() != nil`, true only for a closed context. A failed query reaches the loop as an `invalidMetric` with the context open, so it was appended. The cache was not empty, and later scrapes replayed the error.

## Why the gap existed

The comment and the context check came in October 2023, in [PR #355](https://github.com/burningalchemist/sql_exporter/pull/355), a fix for a cached deadline error. The check covered a closed context, not a failed query.

## The fix

Send an invalid metric on to the scrape and skip the append. The hunk from `collector.go`:

```diff
@@ -158,6 +158,10 @@ func (cc *cachingCollector) Collect(ctx context.Context, conn *sql.DB, ch chan<-
 					ch <- NewInvalidMetric(errors.Wrap(cc.rawColl.logContext, ctx.Err()))
 					continue
 				}
+				if _, invalid := metric.(invalidMetric); invalid {
+					ch <- metric
+					continue
+				}
 
 				cc.cache = append(cc.cache, metric)
 				ch <- metric
```

A collection that only failed leaves the cache empty, so the next scrape runs it again. Metrics from other queries are still cached. The cost: a collector whose queries all keep failing now runs on every scrape.

With `enable_query_metrics: true`, the failed query's duration and row gauges are still cached. Only the error replay stops there, and the query still waits for `min_interval`.

## The test

`TestMinIntervalDoesNotCacheQueryError` in `exporter_test.go` builds an exporter with `min_interval: 1h` and a query on a CSV table that does not exist, read through the csvq driver. The first scrape must fail. The test then creates the table, and the second must return metric `m` with no error. It needs no database server.

Before the fix the test failed in 10 of 10 runs with `second scrape: ... file t does not exist`. After it, 10 of 10 passed.

## What the maintainer did

@burningalchemist approved with no review text and commented: “Seems to be an edge case (with 1h interval making it quite visible). But yes, a necessary fix. Thanks! ☺️” It was my seventh merged sql_exporter PR, after [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115) to [#1120](https://github.com/burningalchemist/sql_exporter/pull/1120).

## The lesson

- A cache must know the difference between a result and an error. An error that gets in outlives the failure.
- Check a comment that states an invariant against the condition under it. This one said invalid metrics; the condition tested a closed context.
- Test the recovery: fail once, remove the cause, then scrape again.

If a cache in your own code stores whatever it is handed, reply on [X](https://x.com/spicysharkco).
