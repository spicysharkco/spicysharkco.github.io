---
layout: post
title: "A copy that shared the original's hook lists: with_options in Prefect"
date: 2026-10-19
project: Prefect
pr: https://github.com/PrefectHQ/prefect/pull/23442
status: merged
tags: [observability-data, python, config]
---

In Prefect, a Python workflow orchestration library, `with_options` made a copy of a task or flow that shared a hook list with the original whenever that list already held a hook. A hook added to the copy was also added to the original, and the other way round. I fixed it in [PR #23442](https://github.com/PrefectHQ/prefect/pull/23442), merged about eleven hours after I opened it.

## When it happens

The issue's script, task half only:

```py
from prefect import task

def log_failure(task, task_run, state): pass
def alert(task, task_run, state): pass

@task(on_failure=[log_failure])
def base_task(): pass

noisy_task = base_task.with_options(name="noisy")
noisy_task.on_failure(alert)

print([h.__name__ for h in base_task.on_failure_hooks])
print([h.__name__ for h in noisy_task.on_failure_hooks])
```

Without the fix both lines print `['log_failure', 'alert']`; with it the first prints `['log_failure']`. A flow did the same.

## How with_options builds the copy

`with_options` called the constructor with `on_failure or self.on_failure_hooks`. The constructor stores `on_failure or []`: a non-empty list stays the same object, an empty one is replaced. So once the original had a hook, both objects held one list, and `on_failure(fn)` appended to it in place. `MaterializingTask.with_options`, behind `@materialize`, reads each argument with `getattr(self, attr_name)`, so it passed on all five task hook lists.

## Why the gap existed

On 5 October 2026 a change to `Task.with_options` made the copy carry `on_rollback` and `on_commit` over, as `copy(self.on_rollback_hooks)` and `copy(self.on_commit_hooks)`. Its assertions compare the copy's lists with `==`, which a shared list also satisfies. The other lists kept the form `on_x or self.on_x_hooks`.

## The fix

The `tasks.py` diff without context lines:

```diff
@@ -842,9 +842,9 @@ class Task(Generic[P, R]):
-            on_completion=on_completion or self.on_completion_hooks,
-            on_failure=on_failure or self.on_failure_hooks,
-            on_running=on_running or self.on_running_hooks,
+            on_completion=on_completion or copy(self.on_completion_hooks),
+            on_failure=on_failure or copy(self.on_failure_hooks),
+            on_running=on_running or copy(self.on_running_hooks),
@@ -2258,9 +2258,10 @@ class MaterializingTask(Task[P, R]):
-            init_kwargs[param_name] = task_kwargs.get(
-                param_name, getattr(self, attr_name)
-            )
+            value = getattr(self, attr_name)
+            if attr_name.endswith("_hooks"):
+                value = copy(value)
+            init_kwargs[param_name] = task_kwargs.get(param_name, value)
```

`Flow.with_options` gets the same `copy(...)` on its five hook lines.

## The test

Three parametrized tests, one row per hook list: `test_hooks_are_copied_from_original_task` (three rows), `test_hooks_are_copied_from_original_flow` (five) and, for `@materialize`, `test_with_options_hooks_are_copied_from_original_task` (five). Each row gives the original one hook, adds a second to the copy with its decorator, and asserts the original still holds one and the copy two. With the old `tasks.py` and `flows.py`, all 13 rows fail; with the fix, all 13 pass.

## What the maintainer did

@desertaxle approved with "LGTM!". It was merged the same day, with the code and tests unchanged.

## The lesson

- A fallback `x or []` hides this bug when the original has no hooks: the constructor builds a fresh list. Give the original a hook before you copy it.
- Search for the method, not the field. `MaterializingTask.with_options` reads each list with `getattr(self, attr_name)`, so a search for `self.on_failure_hooks` finds `Task.with_options` and misses it.

If a copy method in your own code hands a list or dict to a constructor, reply on [X](https://x.com/spicysharkco).
