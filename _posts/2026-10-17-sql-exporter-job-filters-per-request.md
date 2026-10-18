---
layout: post
title: "Request input stored on a shared exporter: job filters in sql_exporter"
date: 2026-10-17
project: sql_exporter
pr: https://github.com/burningalchemist/sql_exporter/pull/1116
status: merged
tags: [observability-data, concurrency, prometheus]
---

Two scrapes of sql_exporter could ask for different jobs and receive each other's metrics. I fixed the handler's shared filter state in [PR #1116](https://github.com/burningalchemist/sql_exporter/pull/1116), merged 2026-10-17, about 4 hours and 7 minutes after I opened it. The filter was request input, but the handler stored it on the exporter that all requests shared.

## When it happens

A job groups database targets and the collectors to run against them. The `jobs[]` query parameter selects which configured jobs to scrape. It can appear more than once. With jobs named `a` and `b`, `/metrics?jobs[]=a` asks for `a`; omitting the filter selects all targets.

The failure needs overlapping requests. One scrape asks for `a` while another asks for `b` on the same exporter. This can happen when separate Prometheus scrape jobs select different jobs from that exporter. A response for `b` could include `scrape_errors_total` series labelled `job="a"`. Less often, the wrong job's `up` and `scrape_duration_seconds` series appeared too. My previous sql_exporter post covered the warning fixed in [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115). Here, the problem reached the response's metrics.

## How a request travels

In `cmd/sql_exporter/main.go`, the exporter is constructed and passed into `ExporterHandlerFor`. The handler lives in `cmd/sql_exporter/promhttp.go`. Before my change, it read `req.URL.Query()["jobs[]"]` and called `exporter.SetJobFilters(jobFilters)` on that shared exporter.

The next step looked reassuring: `exporter.WithContext(ctx)` supplied a copy for gathering. In `exporter.go`, the interface describes that copy as single use. But its constructor copies `jobFilters` from the original. Another request could change that field after the setter and before the copy. The copy then carried the wrong selection into `filteredTargets`, which chooses the database targets to collect.

There was another read later. The handler gathered metrics from the copy and the registry, then called `exporter.FilterScrapeErrorsTotal(mfs)` on the original exporter. That method filters the error counter's series by their `job` labels. Another request could replace the filter during gathering, so even a request that collected the right targets could select the wrong error series at the end.

I found two chances to use another request's input: before copying, and before filtering the error counter. The setter also wrote `jobFilters` without a lock while other requests read it. The PR's race-detector run reported a data race.

## Why the gap existed

[PR #433](https://github.com/burningalchemist/sql_exporter/pull/433) added `jobs[]` selection and the setter-before-copy order. [#915](https://github.com/burningalchemist/sql_exporter/pull/915) added the later error-counter filter. [#916](https://github.com/burningalchemist/sql_exporter/pull/916) made target selection return a filtered slice instead of replacing the exporter's target slice.

Returning a filtered target slice did not make the handler's `jobFilters` request-local. The handler still wrote that field on the shared object and read it again later.

## The fix

I took the copy before setting the filter, then used that same copy at both consumers. These are the changed calls, with intervening code omitted:

```diff
+exp := exporter.WithContext(ctx)
-if err := exporter.SetJobFilters(jobFilters); err != nil {
+if err := exp.SetJobFilters(jobFilters); err != nil {
-gatherer := prometheus.Gatherers{exporter.WithContext(ctx), registry}
+gatherer := prometheus.Gatherers{exp, registry}
-mfs = exporter.FilterScrapeErrorsTotal(mfs)
+mfs = exp.FilterScrapeErrorsTotal(mfs)
```

The handler now sets the request's selection on `exp`, gathers from `exp`, and filters the error counter with `exp`. In multi-job mode, an unknown job name still returns a 400 response; single target mode still ignores job filters. `WithContext` makes a shallow copy, but `SetJobFilters` assigns the filter field on that copy rather than changing a shared slice's contents.

## The test

I added `TestExporterHandlerJobFilters` in `cmd/sql_exporter/promhttp_test.go`. It runs two goroutines, each making up to 10 requests through the same handler and stopping early if a check fails. One selects `a`, the other `b`. The database targets are deliberately unreachable: `up` and `scrape_errors_total` still carry job labels, so the test can check selection without a working database.

Each response must contain its own job and contain none of the other job. Checking only for foreign series would let an empty error response pass.

My PR records the local before/after results: before the fix, 20 of 20 runs failed; after it, 20 of 20 passed. The race-detector run with `-count=5` failed before and passed after. The full suite, its race-detector run, formatting, and vet also passed after the fix. These are the checks recorded with the PR, rather than fresh runs for this post.

## What the maintainer did

@burningalchemist approved with “Great catch! Thanks a lot! 👏” and merged 1 minute and 36 seconds later. There were no inline review comments or issue-thread comments. The PR was one commit.

## The lesson

- Find the first shared write, not just the later read. Copying after that write is too late to isolate requests.
- Follow the selection through every consumer. Correct target collection does not prove the later response filter used the same selection.
- Assert that the expected data is present as well as foreign data absent. An empty response should not pass an isolation test.

If request input escaped into another response in your own code, reply on [X](https://x.com/spicysharkco).
