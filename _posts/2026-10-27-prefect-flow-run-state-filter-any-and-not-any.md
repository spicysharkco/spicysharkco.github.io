---
layout: post
title: "Two fields in one filter, and the filter was dropped: flow run states in Prefect"
date: 2026-10-27
project: Prefect
pr: https://github.com/PrefectHQ/prefect/pull/23459
status: merged
tags: [observability-data, python, testing]
---

In Prefect, a flow run state filter whose `type` or `name` part set both `any_` and `not_any_` was ignored, and the server matched every flow run. I filed [issue #23458](https://github.com/PrefectHQ/prefect/issues/23458) and sent the fix ([PR #23459](https://github.com/PrefectHQ/prefect/pull/23459), merged 2026-10-27 UTC, about a week after I opened it). The diff changes two lines in `src/prefect/server/schemas/filters.py`.

## When it happens

Take one Completed run and one Failed run. A state filter with `any_=["COMPLETED", "FAILED"]` and `not_any_=["FAILED"]` should return only the Completed run. It returned both. The name part behaved the same. With only `any_` or only `not_any_` the filter worked.

The issue adds a worse case: `POST /flow_runs/bulk_delete` takes the same filter. With one Running and one Completed run, I sent a state type filter with `any_` set to RUNNING and COMPLETED and `not_any_` set to COMPLETED. It deleted both runs, and only the Running run matches.

## Why the gap existed

`FlowRunFilterState._get_filter_list` called `as_sql_filter()` on its `type` and `name` parts and kept the result only if it was a `BinaryExpression`. A part with one field gives one comparison, which passes. A part with both gives an `and_` of two comparisons, which is not a `BinaryExpression`, so it was skipped. The issue notes that before commit [3d9e9db81](https://github.com/PrefectHQ/prefect/commit/3d9e9db81) this method extended its list with each part's own filter list, which kept both comparisons. The sources do not say why the check was written that way.

## The fix

The check now skips a part only when it sets nothing, which is `sa.true()`. Both parts changed the same way:

```diff
-            if isinstance(filter, sa.BinaryExpression):
+            if not isinstance(filter, sa.True_):
```

Each part stays one group. Spreading its two comparisons into the outer list would be wrong under `operator="or_"`: it would match every flow run. `TaskRunFilterState` has the same kind of check, but its parts only have `any_`, so it is not affected.

## The test

`test_read_flow_runs_filters_by_states_any_and_not_any` creates three flow runs: Running, Completed and Failed (named `RIP`). It reads them three times. A type filter and a name filter that each set both fields must return only the Running run:

```python
any_=["RUNNING", "COMPLETED"], not_any_=["COMPLETED"]
```

The third read puts both parts under `or_`, a type part with both fields and a name part `any_=["RIP"]`. It expects the Running and Failed runs, which is what a grouped part gives:

```python
assert {res.id for res in result} == {flow_run_1.id, flow_run_3.id}
```

The PR description says `pytest tests/server/models/test_flow_runs.py -k states_any_and_not_any -n 0` gave 1 failed before the fix and 1 passed after.

## What the maintainer did

@desertaxle approved it on 2026-10-27 and merged it, with no review comments. The Devin review bot reported no issues, and CodSpeed reported no change in performance.

## The lesson

- An `isinstance` check on a SQL expression tests its shape, not whether it filters anything. Test none, one and two fields set.
- When parts are joined by a configurable operator, keep each part one group, and test both operators.
- Look at who consumes the filter. A read filter that matches too much is a deletion bug when `bulk_delete` takes it.

If a filter builder of yours keeps a clause by its type, reply on [X](https://x.com/spicysharkco).
