---
layout: post
title: "An unset strategy that nothing defaulted: RayClusterFleet in AIBrix"
date: 2026-10-19
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3169
status: merged
tags: [model-serving, go, testing]
---

A RayClusterFleet in AIBrix, a control plane for serving LLMs on Kubernetes, never created a RayClusterReplicaSet when its strategy was unset. I fixed it in [PR #3169](https://github.com/vllm-project/aibrix/pull/3169), merged about six and a half hours after I opened it.

## When it happens

Four forms of `spec.strategy`, run on the base commit against a test API server. `{}` (what a typed Go client always sends) or omitted: no error and no ReplicaSet. Only `type: RollingUpdate`: a nil pointer panic. `rollingUpdate: {}`: "nil value for IntOrString".

## How the rollout switch works

`Reconcile` switches on `Strategy.Type`. Only Recreate and RollingUpdate have a case, so an empty type returns nil (`rayclusterfleet_controller.go:175,182`). The rolling path dereferences `RollingUpdate.MaxSurge` and `MaxUnavailable` (`util/fleet.go:452`, `:473`, `:811`). Lines are at the base commit. A `+kubebuilder:default` marker sets a default in the CRD schema; the API server applies it only when the key is absent.

## Why the gap existed

The field copies `DeploymentSpec.Strategy`, whose defaults come from Go code a CRD does not run (`SetDefaults_Deployment`). Three sibling fields had markers, `strategy` none; the old controller tests always set a type and both values.

## The fix

The new marker covers the omitted form. `Reconcile` switches on `util.StrategyType(f)`, and each empty field is filled at the read site. Trimmed: doc comments, the import, four more call sites, both CRD copies, the test files and the `if` block copying set fields over defaults. Hunk headers are file names.

```diff
@@ rayclusterfleet_types.go @@
+	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: "25%", maxUnavailable: "25%"}}
@@ util/fleet.go @@
+func StrategyType(deployment *orchestrationv1alpha1.RayClusterFleet) appsv1.DeploymentStrategyType {
+	if deployment.Spec.Strategy.Type == "" {
+		return appsv1.RollingUpdateDeploymentStrategyType
+	}
+	return deployment.Spec.Strategy.Type
+}
+func rollingFenceposts(deployment *orchestrationv1alpha1.RayClusterFleet) (maxSurge, maxUnavailable *intstrutil.IntOrString) {
+	surge, unavailable := intstrutil.FromString("25%"), intstrutil.FromString("25%")
+	maxSurge, maxUnavailable = &surge, &unavailable
+	return maxSurge, maxUnavailable
+}
```

## The test

`TestStrategyDefaults` has six rows, each at 20 replicas. Four envtest specs (a real API server) create the fleet as untyped JSON so a key can be absent, run `Reconcile` once and expect one ReplicaSet. On the base commit all 4 new specs and the table fail, one spec by panic; with the fix all 5 specs pass. Without the marker the omitted spec fails on its stored default. With a 30% surge the table fails: `MaxSurge = 6, want 5`.

## What the maintainer did

@googs1025 approved it and wrote "thanks!". The squash has the same patch as my branch.

## The lesson

- A default marker fills only an absent key: `{}` and a lone type stay as sent. A field inside an imported type cannot carry one, so default where the controller reads it.
- List each way a field can be empty and run each. These four failed three ways (silent, panic, error); the marker fixed one.
- Test that a set value beats the default: with set values ignored, the `maxSurge only` and `both set` rows fail.

If your CRD relies on a default for a nested field, reply on [X](https://x.com/spicysharkco).
