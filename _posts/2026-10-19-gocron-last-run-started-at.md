---
layout: post
title: "Run start reported as run end: LastRunStartedAt in gocron"
date: 2026-10-19
project: gocron
pr: https://github.com/go-co-op/gocron/pull/967
status: merged
tags: [observability-data, go, datetime, testing]
---

With `WithIntervalFromCompletion()`, `Job.LastRunStartedAt()` returned the time the last run ended. gocron is a job scheduler for Go. [PR #967](https://github.com/go-co-op/gocron/pull/967) fixes it, and @JohnRoesler merged it about one day and twenty-one hours after I opened it.

## When it happens

The getter's doc comment uses a ten-minute run as its example: start and completion are ten minutes apart. With the option, the getter returned a time a fraction of a millisecond from `LastRunCompletedAt()`. Without the option it gave the start.

## How the scheduler records a run

A job holds three times: `lastRun`, `lastRunStartedAt` and `lastRunCompletedAt`. The executor sends the last two around the task function. `lastRun` is set when the executor reports the run (`selectExecJobsOutCompleted`): before the task function by default, after it with the option, because the next run is counted from `lastRun`. So it marks the start by default and the end with the option.

## Why the gap existed

`LastRunStartedAt` and the `lastRunStartedAt` field arrived together in April 2026. The getter returned `lastRun`, which already existed; the option dated from October 2025. The only test-file mention was `ExampleJob_lastRunStartedAt`, which has no `// Output:` line, so Go compiles it and never runs it.

## The fix

The `job.go` diff, trimmed to the changed lines (context cut):

```diff
@@ -1768,7 +1768,7 @@ func (j job) LastRunStartedAt() (time.Time, error) {
-	return ij.lastRun, nil
+	return ij.lastRunStartedAt, nil
```

`IsRunning` already reads `lastRunStartedAt`; `lastRun` is untouched, so scheduling and `LastRun()` behave as before.

## The test

`TestJob_LastRunStartedAt` runs a one-hour `DurationJob` with the option on a fake clock; the task advances it by one minute. After `LastRunCompletedAt()` is set, the test expects `LastRunStartedAt()` to equal the starting clock time and the completion to be one minute later. Without the fix the getter returned the start plus one minute. Run three times each way, it failed three of three without the fix and passed three of three with it.

## What the maintainer did

@JohnRoesler approved it, merged `v2` into the branch, and squash-merged it about seventeen minutes after the approval. The merged diff equals my commit's diff.

## The lesson

- When two readers share a field, ask which event it records on each path. `lastRun` meant the start by default and the end with the option.
- Give the test a run of known length. With a task that does not move the fake clock, start and end are equal and the test passes without the fix. A task that advances it by one minute makes them exactly one minute apart, with no sleep.

If a getter of yours reads a field the scheduler also uses to plan the next run, reply on [X](https://x.com/spicysharkco).
