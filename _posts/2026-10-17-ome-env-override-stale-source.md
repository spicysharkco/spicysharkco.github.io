---
layout: post
title: "An environment override kept the source it was meant to replace in OME"
date: 2026-10-17
project: OME
pr: https://github.com/ome-projects/ome/pull/1231
status: merged
tags: [model-serving, environment-variables, testing]
---

OME could replace an environment variable's text and leave its old source attached. A ConfigMap-backed entry then carried both a literal value and `valueFrom`, making the rendered workload invalid. I fixed the matching-entry assignment in [PR #1231](https://github.com/ome-projects/ome/pull/1231), merged 2026-10-17, about 59 minutes after I opened it.

## When it happens

An environment entry can provide a literal through `value`, or obtain its value through `valueFrom`, such as a ConfigMap or Secret reference. I used a ConfigMap reference in the regression test. The troublesome transition was from a sourced entry to a computed literal with the same name.

For the source trace below, I use `spec.deploymentMode: RawDeployment`, a runner with a recognized accelerator resource count of one, and no leader or worker blocks. The input is `PARALLELISM_SIZE` using `valueFrom.configMapKeyRef` in `spec.engine.runner.env`. With no `tensorParallelismOverride` in the runtime's matched accelerator configuration, OME computes the literal `"1"`.

That overwrite was intentional. [PR #1168](https://github.com/ome-projects/ome/pull/1168) documented that the computation replaces any existing `PARALLELISM_SIZE` in the runner environment. I was fixing how that promise was implemented, rather than adding a new precedence rule.

## How the value travels

The service schema in `config/crd/full/ome.io_inferenceservices.yaml` accepts the runner's ConfigMap reference. In `pkg/controller/v1beta1/inferenceservice/utils/merging.go`, runtime engine defaults are merged with the service's engine overrides. The service's values take precedence.

Rendering then reaches `EnginePodSpec` in `pkg/render/engine.go`. With no leader block, it selects the top-level engine runner. It applies the environment updates, merges resources, and applies accelerator argument overrides. When no accelerator parallelism override owns the setting, it calls `SetParallelismEnvVar`.

That function lives in `pkg/render/accelerator.go`. It computes the parallelism size from the recognized resource count and the primary and worker counts. It builds a literal `EnvVar` named `PARALLELISM_SIZE` and passes it to `UpdateEnvVars` in `pkg/controller/v1beta1/inferenceservice/utils/container.go`.

The old helper searched the container's environment by name. On a match, it assigned only `container.Env[i].Value`. The ConfigMap reference lived in a different field, `ValueFrom`, so it survived. The entry now said to use `"1"` and to read a source. A Secret reference in the same field would survive that assignment too; the added regression test specifically covers the ConfigMap case.

## Where it fails

In `pkg/controller/v1beta1/inferenceservice/reconcilers/deployment/deployment_reconciler.go`, the rendered pod spec becomes the Deployment's pod template. For a new Deployment, the reconciler submits a create request. For an existing one, it first attempts a dry-run update, where validation can reject the template before the normal update.

The validation failure documented in my PR is “may not be specified when `value` is not empty”. It rejects `valueFrom` beside a nonempty literal at the pod template. In this RawDeployment path, the Deployment itself fails admission; I would look there before looking for a container startup failure.

The reconciler returns the error. In `pkg/controller/v1beta1/inferenceservice/controller.go`, a non-conflict component reconciliation error becomes a `ComponentReconcileError` warning event on the InferenceService. That connects the small helper assignment to the service-level symptom. My evidence here is the source trace and the PR's tests; I did not validate a live deployment.

## Why the gap was easy to miss

The existing update test started with a literal and replaced it with another literal. There was no old source to clear. The helper's append branch already copied the whole incoming entry, so adding a name and replacing a name did not use the same rule.

## The fix

```diff
-// If it exists, update its value.
-container.Env[i].Value = envVar.Value
+// If it exists, replace it.
+container.Env[i] = *envVar
```

The replacement copies `Name`, `Value`, and `ValueFrom` together. A literal override has no source pointer, so replacing the entry clears the old one. The search still stops at the first matching name, and the append branch is unchanged.

## The test

I added a row to `TestUpdateEnvVars` in `pkg/controller/v1beta1/inferenceservice/utils/utils_test.go`. It starts with `TEST_VAR` reading key `value` from ConfigMap `settings`, then overrides it with `new-value`. The expected entry contains the literal and no source.

The results recorded in my PR isolate the change: with only the helper reverted, 1 row failed and 4 passed. With the fix, all 5 passed. The scoped suites passed with 240 leaf cases in `utils` and 71 in `render`, excluding parent test groups. Those are the PR's local results, not fresh runs for this post.

I also recorded a validation limit. The pinned full linter could not finish type checking because of incompatible dependency export data. Its fast mode passed. I did not claim a full lint pass or a full-suite run.

## What happened in review

A review bot reported no actionable comments, but separately warned about docstring coverage and flagged redundant comments. GitHub records no inline review comments or approval reviews. @pallasathena92 merged the one-commit PR.

## The lesson

- When fields describe alternative ways to supply a value, replace the choice together. Updating the text alone can preserve an obsolete source.
- Test a change of representation: source to literal, not only literal to literal.
- Follow the rendered value to the first workload API that validates it. The helper's return is not the end of the failure path.

If an override in your own code left an old source attached, reply on [X](https://x.com/spicysharkco).
