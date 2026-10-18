---
layout: post
title: "Strategic merge kept both halves of an env entry: a merge fix in OME"
date: 2026-10-17
project: OME
pr: https://github.com/ome-projects/ome/pull/1234
status: merged
tags: [model-serving, environment-variables, strategic-merge]
---

An InferenceService could override a runtime's environment variable and still inherit half of the old one. The merged entry carried a literal `value` beside a `valueFrom` source, or two sources at once. Kubernetes rejects such an entry, and in the default mode the Deployment create or update failed. I fixed the merge in [PR #1234](https://github.com/ome-projects/ome/pull/1234), merged 2026-10-17, about 10 minutes after I opened it.

## When it happens

A bundled runtime sets a literal variable. In `config/runtimes/vllm/llama-3-3-70b-instruct-rt.yaml` it is `SERVED_MODEL_NAME: "vllm-model"`. An InferenceService overrides the same name in `spec.engine.runner.env` and reads the value from a ConfigMap instead. The user wrote one field. The merged entry has two:

```json
{"name":"A","value":"base","valueFrom":{"configMapKeyRef":{"key":"value","name":"settings"}}}
```

The reverse fails too: a runtime entry with `valueFrom` replaced by a literal, or a ConfigMap source replaced by a Secret source. My PR quotes the Kubernetes messages. A literal beside a source gives “may not be specified when `value` is not empty”. Two sources give “may not have more than one field specified at a time”. The default deployment mode is `RawDeployment` (`config/configmap/inferenceservice.yaml`), and its reconciler returns the error from the Deployment create or update.

## How the merge works

`MergeEngineSpec`, `MergeDecoderSpec` and `MergeRouterSpec`, all in `pkg/controller/v1beta1/inferenceservice/utils/merging.go`, share a generic helper, `mergeSpec`. The render path merges pod containers with `MergeRuntimeContainers`. Both turn the runtime and the override into JSON and call `strategicpatch.StrategicMergePatch`.

For a list such as `env`, strategic merge joins entries by `name`, then merges their fields. A field the override leaves out keeps the runtime's value. That suits most fields, because an override can change one field of an entry without restating the others. Here it is wrong. The runtime's `value` stays beside the override's `valueFrom`, and a ConfigMap reference stays beside a Secret reference.

## Why the gap existed

The `StrategicMergePatch` calls have been in `merging.go` since 2025-05-23. Before this change, `merging_test.go` had 993 lines and no case with an environment entry.

I had just fixed the same symptom in `UpdateEnvVars` in [PR #1231](https://github.com/ome-projects/ome/pull/1231), a helper that assigned only `Value`. That PR did not touch `merging.go`, so this second path stayed open.

## The fix

My first attempt marked the override's `valueFrom` with `$patch: replace`. That fixed a ConfigMap replaced by a Secret. When the runtime entry held only a literal, the override's `valueFrom` vanished and the literal stayed. Explicit nulls work instead, because strategic merge deletes a key whose patch value is null.

Before each merge, `nullStaleEnvSources` walks the override JSON. For every `env` entry it nulls the other half. Inside `valueFrom` it nulls every source the entry does not set. This is that part of the new function in `merging.go`:

```diff
+_, hasSource := entry["valueFrom"]
+_, hasValue := entry["value"]
+if hasSource && !hasValue {
+	entry["value"] = nil
+} else if hasValue && !hasSource {
+	entry["valueFrom"] = nil
+}
+if source, ok := entry["valueFrom"].(map[string]any); ok {
+	for _, key := range []string{"fieldRef", "resourceFieldRef", "configMapKeyRef", "secretKeyRef", "fileKeyRef"} {
+		if _, set := source[key]; !set {
+			source[key] = nil
+		}
+	}
+}
```

An entry that sets both fields is left as written. The function runs in `mergeSpec` and in `MergeRuntimeContainers`, so the runner, the leader and worker runners, and pod containers all get it. A new entry is not affected. Its null keys unmarshal to an empty `Value` and nil pointers.

## The test

`TestMergeEnvSources` in `merging_test.go` has four rows: literal replaced by source, source replaced by literal, ConfigMap source replaced by Secret source, and new entries. Each row runs through `MergeEngineSpec`, for the runner and the worker runner, and through `MergeRuntimeContainers`.

With only `merging.go` reverted, three rows fail and the new-entries row passes. The failing output shows the runtime's `Value: "base"` beside the ConfigMap source. With the fix, all four rows pass. I re-ran both cases for this post. My PR also lists five scoped package groups passing and a clean `go vet`. I did not run `make test`.

## What the maintainer did

@slin1237 merged the PR. GitHub shows no reviews and no review comments. The only comment was an automated summary with nothing actionable. Ten checks passed and four were skipped.

My PR also says that `pkg/runtimeinheritance/merge.go` strategic-merges a child runtime over a parent the same way, so it may have the same gap. I left that for a separate change.

## The lesson

- A merge that joins entries by key and then merges fields cannot know that two fields exclude each other. List those pairs and test each one.
- When a fix closes one path to a bad value, search for the other paths that build the same value.
- Try a patch directive on the case where the base lacks the key. `$patch: replace` worked until the runtime entry had nothing to replace.

If a merge in your own code keeps both halves of a choice, reply on [X](https://x.com/spicysharkco).
