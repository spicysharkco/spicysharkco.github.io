---
layout: post
title: "A log line that read the wrong error: the API server certificate check in Kamaji"
date: 2026-10-19
project: Kamaji
pr: https://github.com/clastix/kamaji/pull/1363
status: merged
tags: [kubernetes, go, logging]
---

A one-line mistake in a log call made Kamaji's reconcile panic when `apiserver.crt` was missing from the API server certificate Secret. Kamaji runs Kubernetes control planes as pods. I fixed it in [PR #1363](https://github.com/clastix/kamaji/pull/1363), merged about two and a half hours after I opened it, for [issue #1362](https://github.com/clastix/kamaji/issues/1362).

## When it happens

Remove the certificate from the Secret:

```sh
kubectl patch secret <tcp-name>-api-server-certificate --type=json \
  -p '[{"op":"remove","path":"/data/apiserver.crt"}]'
```

Kamaji should generate a new certificate. Instead the reconcile panics with a nil pointer dereference. controller-runtime recovers the panic, but every requeue panics again. The handlers after this one (other certificates, kubeconfigs, Deployment, Ingress) do not run until someone repairs the Secret. I traced this through the code and a fake-client test, not a live cluster.

## How the certificate check works

`APIServerCertificate` fills the Secret in `mutate`. For an existing Secret it checks the certificate against the CA, the certificate and key as a pair, and the certificate's names (SANs) against what the control plane needs. A check that errors is logged. If any check fails, it generates a new pair.

## Why the gap existed

The key-pair check reports an empty certificate as `false` with a nil error. So with no certificate `err` is nil, the names check fails parsing empty bytes, and its log line called `err.Error()` on that nil. The names check and its log line arrived together in December 2024. The log lines above it print `err`, which suits their calls. This one's call sets `dnsErr`. No test reached this log line.

## The fix

The log line read the wrong error. Print `dnsErr`, the error of the call the `if` tests. The diff is trimmed: no context lines, no test file, no function name in the hunk header.

```diff
@@ -159 +159 @@
-				logger.Info(fmt.Sprintf("%s SAN check returned an error: %s", kubeadmconstants.APIServerCertAndKeyBaseName, err.Error()))
+				logger.Info(fmt.Sprintf("%s SAN check returned an error: %s", kubeadmconstants.APIServerCertAndKeyBaseName, dnsErr.Error()))
```

## The test

A new `api_server_certificate_test.go` has one spec. The stored Secret holds only an `apiserver.key` with a placeholder value. The spec calls `Define`, then `CreateOrUpdate`, and expects `Updated` and a non-empty `apiserver.crt`.

Of the package's 35 specs, 34 passed and the new one failed without the fix. All 35 passed with it.

## What the maintainer did

@prometherion approved it with "Thanks for spotting this! 🔍". It was squash-merged with my change unchanged.

## The lesson

- A function that reports failure as `false` with a nil error leaves `err` nil. When the `if` tests a later call, print that call's error.
- Start the test from the state the report reaches: a Secret with a key and no certificate, as the `kubectl patch` above leaves it.
- Name the blast radius. The reconcile returns at the first failing handler, so one panic held back every handler after it.

If a failure-path log line in your own code has never run in a test, reply on [X](https://x.com/spicysharkco).
