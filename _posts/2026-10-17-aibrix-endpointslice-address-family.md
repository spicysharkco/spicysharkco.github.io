---
layout: post
title: "I made AIBrix match a ModelAdapter EndpointSlice to its Pods' address family"
date: 2026-10-17
---

AIBrix built every ModelAdapter EndpointSlice with `addressType: IPv4`, even when the Pods had IPv6 addresses. I fixed it in [PR #3136](https://github.com/vllm-project/aibrix/pull/3136), merged the same day, about twelve hours after I opened it. The family was a constant in the builder.

## When it happens

A ModelAdapter is loaded on Pods whose `status.podIP` is an IPv6 address such as `2001:db8::1`. The controller creates a slice for the adapter Service. The slice declares `IPv4` but lists IPv6 addresses, so Kubernetes rejects it and the adapter Service has no endpoints.

I did not run this on an IPv6 cluster. My evidence is the code and the unit test below. There was no existing issue for it.

## How the slice is built

`reconcileEndpointSlice` in `modeladapter_controller.go` fetches the Pods named in `status.instances`, then looks for the slice. If there is none, it calls `buildModelAdapterEndpointSlice` and creates the result. If there is one, it only rewrites `Endpoints`, and `addressType` cannot change on an existing slice.

The builder in `resources.go` copied each `status.podIP` into an endpoint, then set `AddressType: discoveryv1.AddressTypeIPv4` (line 58).

## Why the gap existed

The constant came with the controller refactor in [#135](https://github.com/vllm-project/aibrix/pull/135), in September 2024. The builder's own test used an IPv4 Pod address and did not assert the family.

## The fix

The first backing Pod picks the family. With no Pods it stays IPv4. From `resources.go`, without the `k8s.io/utils/net` import (`netutils`) and the unchanged lines between the two hunks:

```diff
+	addressType := discoveryv1.AddressTypeIPv4
+	if len(pods) > 0 && netutils.IsIPv6String(pods[0].Status.PodIP) {
+		addressType = discoveryv1.AddressTypeIPv6
+	}
+
 	addresses := make([]discoveryv1.Endpoint, 0, len(pods))
@@
-		AddressType: discoveryv1.AddressTypeIPv4,
+		AddressType: addressType,
```

The PR body lists what this does not cover. Backing Pods of mixed families would still need separate slices. The family is chosen when the slice is created. Dual-stack Pods are still listed by their primary address, `status.podIP`.

## The test

`TestBuildModelAdapterEndpointSliceIPv6` in `resources_test.go` builds a slice from one Pod with `2001:db8::1` and checks the family and the address. The existing IPv4 test now asserts the IPv4 family too.

Before the fix the new test failed: expected `IPv6`, actual `IPv4`. After it, it passed. The package run without the envtest suite passes, 25 tests and 39 subtests. The 18 controller specs need envtest binaries I did not have, so they did not run for me. CI on the PR head was green.

## What the maintainer did

@varungup90 left a review that called it “Small, correct fix” and then merged the PR. The review reports that restoring the old `resources.go` with the new tests fails the same way. It also notes that the `len(pods) > 0` guard has no test of its own, and calls that optional. I left the PR as it was.

## The lesson

- A hardcoded value is a claim about every input. Ask what the callers can send.
- Assert the old default before you add a new case. The IPv4 test now checks the family it used to assume.
- Write down what the change leaves out. A slice keeps its family, so mixed-family Pods need more than this.

If a builder in your own code assumes one kind of input, reply on [X](https://x.com/spicysharkco).
