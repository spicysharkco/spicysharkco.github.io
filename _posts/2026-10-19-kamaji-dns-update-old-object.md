---
layout: post
title: "An update checked against the old object: the DNS service IP webhook in Kamaji"
date: 2026-10-19
project: Kamaji
pr: https://github.com/clastix/kamaji/pull/1369
status: merged
tags: [kubernetes, go, webhooks]
---

A TenantControlPlane could be edited to a DNS service IP outside its service CIDRs. Kamaji runs Kubernetes control planes as pods, and its `dnsServiceIPs` webhook checked the old object on update, not the new one. I fixed it in [PR #1369](https://github.com/clastix/kamaji/pull/1369), merged about three hours and fifty-five minutes after I opened it, for [issue #1368](https://github.com/clastix/kamaji/issues/1368).

## When it happens

Take `serviceCidrs` `10.96.0.0/16` and `dnsServiceIPs` `10.96.0.10`. Edit `dnsServiceIPs` to `10.231.4.1`. The webhook admits it, though the address is outside every service CIDR. In kubeadm's rendered output the kubelet `clusterDNS` takes that address while the CoreDNS Service keeps the one derived from the CIDR. I did not run a live cluster.

Edit it back to `10.96.0.10` and the webhook denies it with `DNS service IP "10.231.4.1" is not contained in any configured service CIDR`, naming the address being replaced.

## How update admission works

The chain decodes the new and the old object from the request, then calls `OnUpdate(decodedObj, oldDecodedObj)`. The interface names them `newObject` and `prevObject`. `validateDNSServiceIPs` requires each `dnsServiceIPs` entry to lie inside a `serviceCidrs` range.

## Why the gap existed

The handler was added on 18 June 2026 as `OnUpdate(_, newObject runtime.Object)`. Both parameters are `runtime.Object`, so the compiler accepted it. The existing update specs pass an object and a copy of it, so either order passed.

## The fix

Bind the first parameter. The diff is trimmed: no context lines, no test file, no function name in the hunk header.

```diff
@@ -25 +25 @@
-func (t TenantControlPlaneDNS) OnUpdate(_, newObject runtime.Object) AdmissionResponse {
+func (t TenantControlPlaneDNS) OnUpdate(newObject, _ runtime.Object) AdmissionResponse {
```

Updates are now checked as creates are: an edit to either field that leaves an address outside the CIDRs is denied, and a stored bad value can be corrected.

## The test

Two specs in a new `Context` in `tcp_dns_test.go`, each with an old and a new object holding different addresses. One edits `10.96.0.10` to `10.231.4.1` and expects the error. The other replaces a stored `10.231.4.1` with a valid address and expects none. On the old code the first gets no error and the second is denied. Both pass with the fix. A one-off probe, not in the PR, showed the `serviceCidrs` case admitted before and denied after.

## What the maintainer did

@prometherion approved it with "Such a silly error from my side, thanks for this!" and squash-merged it with my diff unchanged.

## The lesson

- Treat `_` in a method signature as a decision. Check what the interface passes in that slot; here it was the new object.
- Read a denial against the request. The corrective edit was denied for the old address, so the check was reading the stored object.
- Probe an update check once per field it reads. The report edited `dnsServiceIPs`; an edit to `serviceCidrs` was wrong too.

If a webhook in your own project takes an old and a new object, reply on [X](https://x.com/spicysharkco).
