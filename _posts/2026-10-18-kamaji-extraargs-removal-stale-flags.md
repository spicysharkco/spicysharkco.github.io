---
layout: post
title: "A reset that skipped a removed block: kube-apiserver extraArgs in Kamaji"
date: 2026-10-18
project: Kamaji
pr: https://github.com/clastix/kamaji/pull/1360
status: merged
tags: [kubernetes, go, config]
---

Deleting the whole `extraArgs` block from a TenantControlPlane in Kamaji, which runs Kubernetes control planes as pods, left its flags on the kube-apiserver container. I fixed it in [PR #1360](https://github.com/clastix/kamaji/pull/1360), merged about twelve hours and fifty minutes after I opened it. It was my first Kamaji PR.

## When it happens

Add an API server flag, let the Deployment roll out, then delete the block:

```yaml
spec:
  controlPlane:
    deployment:
      extraArgs:
        apiServer:
          - --audit-log-path=/var/log/audit.log
```

The container keeps `--audit-log-path`. Removing only `extraArgs.apiServer` while other `extraArgs` fields stay, or setting `extraArgs: {}`, already reset the args.

## How the args reset works

Each reconcile calls `resetKubeAPIServerFlags` before it sets the containers. It hashes `extraArgs.apiServer` and compares the hash with the Deployment annotation `kube-apiserver.kamaji.clastix.io/args`. If they differ, it empties the container args, which are rebuilt from clean, and stores the new hash.

## Why the gap existed

The function began with `if ... ExtraArgs == nil { return }`. That line dates from July 2022. A nil block means "never set" and also "set, then removed". The annotation tells them apart, but the function read it only after that return. `mergeAPIServerArgs` keeps live flags that Kamaji does not own, so the stale flag survived every pass.

## The fix

With the block gone and the annotation present, hash an empty list, so the existing mismatch path resets the args. Without the annotation the function still returns early. The diff is trimmed: no context lines, no function names in the hunk headers, no test file.

```diff
@@ -1206 +1206,6 @@
-	if tcp.Spec.ControlPlane.Deployment.ExtraArgs == nil {
+	var extraArgs []string
+
+	if tcp.Spec.ControlPlane.Deployment.ExtraArgs != nil {
+		extraArgs = tcp.Spec.ControlPlane.Deployment.ExtraArgs.APIServer
+	} else if _, tracked := resource.GetAnnotations()[apiServerFlagsAnnotation]; !tracked {
+		// no extra args now, and none ever applied: nothing to reset
@@ -1220 +1225 @@
-	currentHash, err := utilities.CalculateStringSliceChecksum(tcp.Spec.ControlPlane.Deployment.ExtraArgs.APIServer)
+	currentHash, err := utilities.CalculateStringSliceChecksum(extraArgs)
```

## The test

Two specs in `deployment_test.go` call `resetKubeAPIServerFlags`. The container holds `--audit-log-path=/var/log/audit.log` and `--secure-port=6443`; the TenantControlPlane has no `extraArgs`. With an annotation holding the audit flag's hash, the args must end empty. With no annotation, both stay.

Without the fix, three runs gave 35 specs, 34 passed and 1 failed, the removal spec. With it, 35 passed. I did not run a live cluster.

## What the maintainer did

@prometherion merged it as a squash of my one commit, with no comment and no change.

## The lesson

- A nil check cannot tell "never set" from "set, then removed". When a reset compares input with stored state, ask what happens once the input is gone.
- When you widen an early return, test the case it must still skip. With the annotation check removed, the never-set spec fails.
- Say which near misses do not trigger it. `extraArgs: {}` keeps a non-nil pointer, so the early return never fired.

If a reset in your own controller returns early on a missing field, reply on [X](https://x.com/spicysharkco).
