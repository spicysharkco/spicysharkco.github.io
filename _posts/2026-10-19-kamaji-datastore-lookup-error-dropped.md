---
layout: post
title: "An error that was logged, not returned: the DataStore lookup in Kamaji"
date: 2026-10-19
project: Kamaji
pr: https://github.com/clastix/kamaji/pull/1365
status: merged
tags: [kubernetes, go, error-handling]
---

Kamaji hosts Kubernetes control planes as pods. When it could not fetch the DataStore a TenantControlPlane points to, its reconcile logged the failure and returned success. I fixed it in [PR #1365](https://github.com/clastix/kamaji/pull/1365), merged about two hours and ten minutes after I opened it, for [issue #1364](https://github.com/clastix/kamaji/issues/1364).

## When it happens

A TenantControlPlane uses a DataStore that is not ready, so `status.storage.dataStoreName` stays empty. Delete that DataStore, wait a few seconds, and recreate it. Its finalizer only waits for TenantControlPlanes listed in that field, so the deletion goes through.

The log shows `cannot retrieve the DataStore for the given instance` with no error attached, and `Reconcile` returns success. The DataStore controller triggers only TenantControlPlanes listed in that field, so this one waits until it is edited or the cache resync fires. I traced this in the code and with a fake-client test, not a live cluster.

## How the lookup works

`Reconcile` fetches the DataStore through `dataStore`, which wraps a failed `Get` with `%w`. `Reconcile` should log that error and return it, so controller-runtime retries with backoff.

## Why the gap existed

The log-and-return pair dates from August 2022, when `err` held the lookup error. On 21 February 2026 a refactor renamed the variable to `dsErr` in the declaration and the `if`. It left the log call and the return on `err`. That `err` is the outer variable from fetching the TenantControlPlane and taking the lock, and it is nil there.

## The fix

Use `dsErr` in both. A failed lookup is now returned, including a DataStore that does not exist. The diff is trimmed: no context lines, no test file, no function name in hunk headers.

```diff
@@ -162 +162 @@
-		log.Error(err, "cannot retrieve the DataStore for the given instance")
+		log.Error(dsErr, "cannot retrieve the DataStore for the given instance")
@@ -164 +164 @@
-		return ctrl.Result{}, err
+		return ctrl.Result{}, dsErr
```

## The test

`TestTenantControlPlaneReconcilerReturnsDataStoreLookupError` is in a new file. A TenantControlPlane names a DataStore that does not exist. A fake client holds only the TenantControlPlane. The test calls `Reconcile` and expects `k8serrors.IsNotFound` to hold for the returned error.

Before the fix `Reconcile` returned a nil error here and the test failed. Now it passes. It asserts the returned error only, not the log call.

## What the maintainer did

@prometherion approved it with "Thanks!". It was squash-merged with my diff unchanged.

## The lesson

- After you rename a variable inside a guard, search the whole block for the old name. Here an outer `err`, always nil, let the compiler accept both stale lines.
- Assert the returned error by kind, not by text. `dataStore` wraps with `%w`, and `k8serrors.IsNotFound` still sees through the wrap.
- A swallowed error leaves two traces: an error log with no error attached, and a reconcile that reports success. Search logs for the first.

If a rename in your own controller left an old variable name behind, reply on [X](https://x.com/spicysharkco).
