---
layout: post
title: "Removed ranges kept on the Service: loadBalancerSourceRanges in Kamaji"
date: 2026-10-19
project: Kamaji
pr: https://github.com/clastix/kamaji/pull/1367
status: merged
tags: [kubernetes, go, networking]
---

Clients outside a TenantControlPlane's old source ranges stayed blocked after the ranges were removed from it. Kamaji runs Kubernetes control planes as pods, and copied `loadBalancerSourceRanges` to the Service only when the list was not empty. I fixed it in [PR #1367](https://github.com/clastix/kamaji/pull/1367), merged about four hours and twenty minutes after I opened it, for [issue #1366](https://github.com/clastix/kamaji/issues/1366).

## When it happens

Take a `LoadBalancer` TenantControlPlane with two `loadBalancerSourceRanges` and remove both. The Service kept them, though the field documentation says an empty list allows all IP ranges.

Change `serviceType` to `ClusterIP` or `NodePort` instead. The CRD rule forces the list empty, so the Service update fails with ``spec.LoadBalancerSourceRanges: Forbidden: may only be used when `type` is 'LoadBalancer'``, and `Reconcile` returns it. The handlers after the Service, such as the Deployment, do not run. I checked this with Service validation and a fake client, not a live cluster.

## How the Service is reconciled

`CreateOrUpdate` reads the live Service through `utilities.CreateOrUpdateWithConflict`, then runs `mutate` on it. A field `mutate` skips keeps its live value.

## Why the gap existed

In October 2024 the copy was added inside the LoadBalancer case, guarded by a non-empty check. An empty list meant "leave what is there".

## The fix

One assignment before the type switch replaces the guarded copy. The diff is trimmed: no context lines, no test file, no function name in hunk headers.

```diff
@@ -143,0 +144,4 @@
+		// Always written, even when empty: removing the ranges from the TenantControlPlane
+		// must clear them on the Service, and the API server rejects them on any other Service type.
+		r.resource.Spec.LoadBalancerSourceRanges = tenantControlPlane.Spec.NetworkProfile.LoadBalancerSourceRanges
+
@@ -167,4 +170,0 @@
-
-			if len(tenantControlPlane.Spec.NetworkProfile.LoadBalancerSourceRanges) > 0 {
-				r.resource.Spec.LoadBalancerSourceRanges = tenantControlPlane.Spec.NetworkProfile.LoadBalancerSourceRanges
-			}
```

## The test

A new `Describe` in `k8s_service_resource_test.go` holds a fake-client LoadBalancer Service with two ranges and runs `CreateOrUpdate`. Three specs: none listed, the Service ends with none; another range listed, it holds only that; type `ClusterIP`, it ends as `ClusterIP` with none. The first and third fail on the old code; the second passes there on purpose, guarding the replacing that already worked. All three pass with the fix.

## What the maintainer did

@prometherion squash-merged it with my diff unchanged.

## The lesson

- Ask which cases a write must reach. Inside the LoadBalancer case, the copy never ran for ClusterIP or NodePort, where the API server rejects leftover ranges.
- Before writing an empty value on every pass, rule out an update loop. Kubernetes sets no default for this field, and controller-runtime's `CreateOrUpdate` compares with `equality.Semantic.DeepEqual`, which equates nil and empty slices.
- When a controller starts owning a field, say who loses. Ranges added to the Service by hand are now removed when the TenantControlPlane lists none.

If a controller of yours copies a field only when it is set, reply on [X](https://x.com/spicysharkco).
