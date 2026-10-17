---
layout: post
title: "I made sql_exporter report a scrape mode change on reload"
date: 2026-10-17
---

A reload that switched sql_exporter from one `target` to `jobs` returned success and kept the old target. I fixed it in [PR #1119](https://github.com/burningalchemist/sql_exporter/pull/1119), merged 2026-10-17, 24 minutes after I opened it. A `case` with no body ended a Go `switch`, and `Reload` returned nil.

## When it happens

The exporter runs with a single `target:`. The config is edited to use `jobs:`, and the exporter gets SIGHUP or a request to `/reload`. The endpoint exists only with `-web.enable-reload`.

Both paths log only two warnings: `Reloading collectors has started...` and `Connections will not be changed upon the restart of the exporter`. No error is logged or returned. `/reload` answers 200, and the scrapes keep serving the old target.

The other direction, `jobs:` to a single `target:`, was refused. The log shows `changing scrape mode is not allowed. Please restart the exporter` and `/reload` answers 500.

## How Reload picks a case

`Reload` in `reload.go` loads the new config, then runs a `switch` on the old and new scrape mode. Target to target calls `reloadTarget`. Jobs to jobs calls `reloadJobs`. The two mode changes look meant to share one error. Anything else logs a warning.

A Go `switch` runs one case and does not fall through. A case with no body ends the switch, and the function goes on to its final `return nil`.

## Why the gap existed

The empty case came with the first version of `reload.go`, in [PR #548](https://github.com/burningalchemist/sql_exporter/pull/548), merged in June 2024. Two stacked cases read as if they share a body. In Go they do not.

The only test of `Reload` I found, `TestReloadMemoryLeak` in `reload_test.go`, reloads one jobs config 500 times behind the `integration` build tag. It never changes the scrape mode.

## The fix

Put both conditions in one `case`. This is the hunk from `reload.go`:

```diff
@@ -38,8 +38,8 @@ func Reload(e Exporter, configFile *string) error {
 		if err = reloadJobs(e, configNext, configCurrent); err != nil {
 			return err
 		}
-	case configCurrent.Target != nil && len(configNext.Jobs) > 0:
-	case len(configCurrent.Jobs) > 0 && configNext.Target != nil:
+	case configCurrent.Target != nil && len(configNext.Jobs) > 0,
+		len(configCurrent.Jobs) > 0 && configNext.Target != nil:
 		return errors.New("changing scrape mode is not allowed. Please restart the exporter")
 	default:
 		slog.Warn("No target or jobs have been found - nothing to reload")
```

The error is the one the jobs-to-target case already returned. Nothing else in `reload.go` changes.

## The test

`TestReloadScrapeModeChange` in `exporter_test.go` writes two configs, one with a single target and one with jobs. It builds an exporter from the first and reloads the second, then does the opposite. Each direction must return an error that contains `changing scrape mode`. It needs no database.

Before the fix, `TargetToJobs` failed with `Reload = <nil>, want a scrape mode change error`, and `JobsToTarget` passed. Over 10 runs, `TargetToJobs` failed 10 times and `JobsToTarget` passed 10 times. After the fix both passed in all 10 runs.

## What the maintainer did

@burningalchemist approved with “🚀” and merged 22 seconds later. It was my fifth merged sql_exporter PR, after [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115), [#1116](https://github.com/burningalchemist/sql_exporter/pull/1116), [#1117](https://github.com/burningalchemist/sql_exporter/pull/1117) and [#1118](https://github.com/burningalchemist/sql_exporter/pull/1118).

## The lesson

- An empty `case` in Go is a silent exit, not a fall-through. To share a body, list both conditions in one `case`, separated by a comma.
- Test both directions of a symmetric rule. Here one direction already worked, so a test of only that direction would have passed.
- Return an error for a request you cannot honour. A success reply says the new config took effect.

If a `switch` in your own code has a case with no body, reply on [X](https://x.com/spicysharkco).
