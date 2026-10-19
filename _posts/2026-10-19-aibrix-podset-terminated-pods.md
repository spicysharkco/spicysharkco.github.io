---
layout: post
title: "Terminated pods that hold their names: PodSet replacement in AIBrix"
date: 2026-10-19
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3146
status: merged
tags: [model-serving, go, testing]
---

A PodSet in AIBrix, a control plane for serving LLMs on Kubernetes, never replaced a pod that ended Failed or Succeeded. I fixed it in [PR #3146](https://github.com/vllm-project/aibrix/pull/3146), merged about twenty-eight hours after I opened it.

## When it happens

An eviction or a node shutdown leaves a pod Failed; with `restartPolicy` `Never` or `OnFailure` it can end Succeeded. The pod stays in the API and the PodSet stays one pod short.

## How the PodSet counts and names pods

A pod's name comes from the PodSet name and its index (`podset_controller.go:407`). `filterActivePods` leaves Failed and Succeeded pods out of the count (`:571-574`), so the controller creates that index again. The create returns `AlreadyExists`, and the controller logs "Pod already exists, skipping" (`:293-294`). Nothing deletes it, so this repeats every reconcile. Lines are at the base commit.

## Why the gap existed

No PodSet test had a Failed or Succeeded pod. `StatefulRoleSyncer` (RoleSet controller) already deletes such pods (`rolesyncer.go:72-75`).

## The fix

At the start of `reconcilePods`, delete each Failed or Succeeded pod, record an event and return. The delete event starts the next reconcile, which creates the replacement. A pod already being deleted is skipped: a finalizer can hold a Failed pod, and each pass would otherwise return before creating the other missing pods.

Three of the 22 added lines, no context: the filter, the delete, the return.

```diff
@@ -227,0 +229,22 @@
+		if pod := &podList.Items[i]; podutil.IsPodTerminal(pod) && pod.DeletionTimestamp == nil {
+		result, err := controllerdrain.DeletePods(ctx, r.Client, r.EventRecorder, podSet, terminatedPods, nil, aibrixconst.PodDrainReasonRollout, time.Now())
+		return result, nil
```

## The test

`TestReconcilePodsReplacesTerminatedPod` runs for Failed and Succeeded, with terminated pods at indexes 1 and 2. After the first pass both are gone and two events exist; after the second both are back. A second test holds a Failed pod at index 1 with a finalizer and checks that index 2 is still created. Without the fix the Failed and Succeeded runs fail and the second test passes; with it the package's 11 tests pass. Removing the deletion-timestamp check fails only the second test.

## What happened in review

@varungup90 left two review comments that recommended approval but were not a formal one. Each reported a mutant that survived the tests. My second commit added the event they suggested and the check that the pod is gone after pass one; the third named the event reason as a constant and added a second terminated pod. @googs1025 approved and merged.

## The lesson

- Make the count and the names agree on what exists. Failed pods left the count but kept their names, and the skip on `AlreadyExists` hid the clash.
- Give a loop two items. With one terminated pod, recording an event for only the first passed; with two, `require.Len(recorder.Events, 2)` fails it.
- Assert the state between passes. A fake client deletes at once, so only a `NotFound` check after pass one tells delete-then-create from both in one pass.

If a create in your own controller skips on `AlreadyExists`, reply on [X](https://x.com/spicysharkco).
