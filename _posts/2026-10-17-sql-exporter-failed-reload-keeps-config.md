---
layout: post
title: "I made sql_exporter keep its config when a reload fails"
date: 2026-10-17
---

After a failed reload, sql_exporter showed a config it was not running. I fixed it in [PR #1120](https://github.com/burningalchemist/sql_exporter/pull/1120), merged within ten minutes of opening it. `Reload` stored the new config before it built the new targets.

## When it happens

The exporter runs with `jobs:` and Prometheus scrapes `/metrics?jobs[]=a`. After an edit, the exporter gets SIGHUP or a request to `/reload`, which exists only with `-web.enable-reload`. The edit renames job `a` to `k`, and a metric gets `key_labels: [v]` next to `values: [v]`.

The config loads, but building the target fails: `column "v" used both as key and value`. The reload returns that error and the old targets keep serving, so `/metrics` still returns job `a`.

But `/config` now shows the rejected config. `/metrics?jobs[]=a` answers 400 `Error setting job filters: invalid job name: a`, for a job that is still running. This lasts until a reload succeeds.

In single target mode a failed reload leaves the new target in `/config`. A refused scrape mode change replaces the collectors there.

## How Reload stores a config

`Reload` in `reload.go` loads the new config and replaces the stored `Collectors` before it touches any target. Then `reloadTarget` sets `cc.Target` before it calls `NewTarget`, and `reloadJobs` sets `cc.Jobs` before it calls `NewJob`. A failure returns before `UpdateTarget`. The `jobs[]` filter and `/config` read the stored config, not the running targets.

## Why the gap existed

All three early writes came with the first version of `reload.go`, in [PR #548](https://github.com/burningalchemist/sql_exporter/pull/548), merged in June 2024. Storing first and building after works until a build fails.

## The fix

Build from the new config, and store it after the build succeeds. This is the hunk for `reloadTarget`:

```diff
@@ -52,15 +49,15 @@ func reloadTarget(e Exporter, nc, cc *cfg.Config) error {
 
 	// Intended: we want to preserve connection details from the previous config. Only collectors will be updated.
 	nc.Target.DSN = cc.Target.DSN
-	// Apply the new target configuration
-	cc.Target = nc.Target
 	// Recreate the target object
-	target, err := NewTarget("", cc.Target.Name, "", string(cc.Target.DSN),
-		cc.Target.Collectors(), nil, cc.Globals, cc.Target.EnablePing)
+	target, err := NewTarget("", nc.Target.Name, "", string(nc.Target.DSN),
+		nc.Target.Collectors(), nil, cc.Globals, nc.Target.EnablePing)
 	if err != nil {
 		slog.Error("Error recreating a target", "error", err)
 		return err
 	}
+	// Apply the new target configuration
+	cc.Target = nc.Target
 
 	// Close old targets before replacing — releases sql.DB pools and sql.Stmts.
 	closeTargets(e.Targets())
```

`reloadJobs` does the same with `cc.Jobs`. In `Reload`, the `Collectors` assignment moves below the `switch`.

## The test

`TestReloadFailureKeepsConfig` in `exporter_test.go` has three rows: a failing target, failing jobs and a refused scrape mode change. Each builds an exporter, reloads a bad file and checks that `Config().YAML()` is unchanged. No database is needed.

Before the fix each row failed in 10 of 10 runs. After it they passed in 10 of 10. Putting back any one of the three early writes fails at least one row. A probe of the HTTP handlers gave 400 for `jobs[]=a` before and 200 after, and `/config` stopped showing job `k`.

## What the maintainer did

@burningalchemist approved with “😃👍” and merged. It was my sixth merged sql_exporter PR, after [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115), [#1116](https://github.com/burningalchemist/sql_exporter/pull/1116), [#1117](https://github.com/burningalchemist/sql_exporter/pull/1117), [#1118](https://github.com/burningalchemist/sql_exporter/pull/1118) and [#1119](https://github.com/burningalchemist/sql_exporter/pull/1119).

## The lesson

- Build the new state first and swap it in only on success.
- Find every reader of stored state. Here `/config` and the `jobs[]` filter read it, not the running targets.
- Test every exit that fails after the input is read. The target row and the jobs row each failed when their early write came back.

If a reload in your own code stores state before it builds, reply on [X](https://x.com/spicysharkco).
