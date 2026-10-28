---
layout: post
title: "An optional field compared as if it were required: Machine UpToDate in Cluster API"
date: 2026-10-27
project: Cluster API
pr: https://github.com/kubernetes-sigs/cluster-api/pull/14375
status: merged
tags: [kubernetes, go, testing]
---

In Cluster API, a Machine without `spec.version` got `UpToDate=False` as soon as its Node reported a kubelet version, and the Cluster stayed in `RollingOut`. I sent the fix ([PR #14375](https://github.com/kubernetes-sigs/cluster-api/pull/14375), merged 2026-10-27). @jakefhyde reported it in [issue #14374](https://github.com/kubernetes-sigs/cluster-api/issues/14374) on 2026-10-15 and suggested the guard. The fix is one added condition and one test row.

## When it happens

The issue's setup: CAPI 1.14.2, a Cluster created without ClusterClass, Machines with no `spec.version`. The Cluster showed `RollingOut`, and the message carried the line `Node.status.nodeInfo.kubeletVersion v1.37.1+rke2r1,  required`. The required version in that line is an empty string. The field is documented in the issue as "meant to be optionally used by bootstrap providers", and it is `+optional`.

## Why the gap existed

The issue traces the check to [commit e903c36](https://github.com/kubernetes-sigs/cluster-api/commit/e903c3615a0edd817f3ba865e8b8f03a9a2d4f6c). Per the issue, its PR text says that if `Machine.status.nodeInfo.kubeletVersion` is set, it is used instead of `Machine.spec.version` for status versions, "to also cover cases where the version differs, e.g. in-place Kubernetes upgrades". With an empty `spec.version`, any non-empty kubelet version differs from it. @salasberryfin put it this way on the issue: "a check on an optional field doesn't take into account the empty case."

## The fix

The check is in `setUpToDateCondition`, in `core/reconcilers/machine/machine_controller_status.go`. The reporter's guess was a `m.Spec.Version != ""` test. That is the change:

```diff
-	if m.Status.NodeInfo != nil && m.Status.NodeInfo.KubeletVersion != "" && m.Status.NodeInfo.KubeletVersion != m.Spec.Version {
+	if m.Spec.Version != "" && m.Status.NodeInfo != nil && m.Status.NodeInfo.KubeletVersion != "" && m.Status.NodeInfo.KubeletVersion != m.Spec.Version {
```

Without a `spec.version` the kubelet check is skipped.

## The test

`TestSetUpToDateCondition` has a new row, "up-to-date, spec.version not set". The Machine reports a kubelet version and has no `spec.version`:

```go
machine: &clusterv1.Machine{
	Status: clusterv1.MachineStatus{
		NodeInfo: &corev1.NodeSystemInfo{
			KubeletVersion: "v1.31.0",
		},
	},
},
```

It expects `UpToDate` to be `True` with reason `clusterv1.MachineUpToDateReason`. The row above it shows the normal message for a real mismatch: `* Node.status.nodeInfo.kubeletVersion v1.30.0, v1.31.0 required`.

## What the maintainer did

- @salasberryfin ran `/ok-to-test` on 2026-10-21 and linked [#14086](https://github.com/kubernetes-sigs/cluster-api/issues/14086), where the discussion was about how the comparison must be applied. Here, they wrote, since `spec.version` is optional it makes sense to skip the check when it is empty.
- @sbueringer asked on the changed line: "I think we have more places like this. Can you take a look please?" I went through the other readers of `spec.version`:
  - KCP has the same kubelet check, but its `spec.version` is required, and a Machine whose version differs from it, empty included, is already marked not up to date in `filters.go` before that check runs.
  - `MachineTemplateUpToDate` compares two templates, so empty against set there is a real template change.
  - The kubeadm bootstrap provider fails on a Machine without a version, with `failed to parse kubernetes version ""`. Fixing that means choosing a kubeadm API version when none is given, so I kept it out of this PR.
- On 2026-10-27 @sbueringer answered: "I think you are right. All good." They had checked a few places too, and this seemed to be the only one that needed a change. They added `/lgtm`, @fabriziopandini added `/approve`, and the bot merged it.

The diff did not change during review: the PR is one commit.

## The lesson

- Every comparison that reads an optional field needs its empty case. Here an empty string was a version that never matches.
- When a reviewer asks "more places like this?", answer with a list that says why each place is safe or out of scope. The thread closed on that list.
- Keep a neighbouring failure out of the fix when it needs a design choice.

If a check in your own code compares against a field that may be empty, reply on [X](https://x.com/spicysharkco).
