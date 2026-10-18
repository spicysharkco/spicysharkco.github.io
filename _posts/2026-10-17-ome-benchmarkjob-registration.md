---
layout: post
title: "A refactor dropped the webhook and controller: BenchmarkJob restored in OME"
date: 2026-10-17
project: OME
pr: https://github.com/ome-projects/ome/pull/1232
status: merged
tags: [model-serving, webhooks, testing]
---

On OME's main branch, creating a BenchmarkJob failed at admission. If it had passed, nothing would have reconciled it. The chart still declared a validating webhook for the kind, but the manager no longer registered that webhook or the controller. I restored both in [PR #1232](https://github.com/ome-projects/ome/pull/1232), merged 2026-10-17, about 2 hours and 25 minutes after I opened it.

## When it happens

Install from `main` and apply a valid BenchmarkJob. My PR records this error:

```text
failed calling webhook "benchmarkjob.ome-webhook-server.validator": failed to call webhook: the server could not find the requested resource
```

The chart template `benchmarkjobvalidator.yaml` and `config/webhook/manifests.yaml` both declare the path `/validate-ome-io-v1beta1-benchmarkjob` with `failurePolicy: Fail`. The API server calls that path, and the manager serves nothing there. With `Fail`, the create is rejected. The chart declares this webhook unconditionally and starts the manager with `--webhook`, so every chart install from `main` hit this. The v1.2.x tags predate #804 and still register both.

## How the manager wires it

`cmd/manager/main.go` sets up each controller. When `options.enableWebhook` is set, it also registers each webhook path on the manager's webhook server with `hookServer.Register`.

## Why the gap existed

[PR #804](https://github.com/ome-projects/ome/pull/804), the InferenceService controller overhaul, was merged on 2026-08-31. Its description listed "The benchmark controller and its config surface" under "Kept as-is." In `cmd/manager/main.go`, the same change removed the controller setup and the webhook registration. The registration was out of `main` for 47 days.

No test in `cmd/manager` checked a webhook path. Both packages still built and kept their own tests. But `go list -deps ./cmd/manager` on `main` lists neither, and no Go file imports either one. The chart kept declaring the path.

## The fix

The controller setup returns to the non-control-plane branch. It builds its event broadcaster with the manager's `newEventBroadcaster(eventRecorderSettings)` helper, and the control-plane log line names BenchmarkJob again. This is the webhook hunk:

```diff
@@ -797,6 +813,11 @@ func main() {
 			Handler: &servingruntime.ServingRuntimeValidator{Client: mgr.GetAPIReader(), Decoder: admission.NewDecoder(mgr.GetScheme())},
 		})
 
+		setupLog.Info("Registering benchmark job validator webhook to the webhook server")
+		hookServer.Register("/validate-ome-io-v1beta1-benchmarkjob", &webhook.Admission{
+			Handler: &benchmark.BenchmarkJobValidator{Client: mgr.GetClient(), Decoder: admission.NewDecoder(mgr.GetScheme())},
+		})
+
 		setupLog.Info("Registering serving runtime engine-preset mutator webhooks to the webhook server")
 		hookServer.Register("/mutate-ome-io-v1beta1-servingruntime-preset", &webhook.Admission{
 			Handler: &runtimepreset.ServingRuntimeMutator{Decoder: admission.NewDecoder(mgr.GetScheme())},
```

Both restored pieces keep their old gates. The controller runs only without the control-plane role. The webhook is registered only when webhooks are enabled.

## The test

A test pinned to one path would catch only this removal. I added `TestDeclaredWebhooksAreRegistered` to `cmd/manager/main_test.go` instead. It parses `main.go` and collects every path passed to `hookServer.Register`. It then reads each `path:` line in the chart's webhook templates and in `config/webhook/manifests.yaml`. For any declared path that `main.go` does not register, it fails and names the file and the path.

A review comment pointed out a weakness in my first version. It accepted a `Register` call on any receiver, so an unrelated call with the same path could keep the test green. Now only `hookServer.Register` calls count. I also stopped hard-coding the InferenceService path. controller-runtime registers that validator itself, so the test now derives its path from the type passed to `NewWebhookManagedBy`.

My PR records the test failing with the registration removed (1 failed, 0 passed) and passing with it (1 passed). The package suites passed with 67 manager, 19 controller and 23 webhook leaf cases.

I also ran both versions of the manager against a single-node cluster, with a serving certificate and a URL-based webhook configuration. The same BenchmarkJob was rejected with the error above on `main`. With the fix, it was admitted and reconciled into an owned Job. I did not test benchmark execution. The pinned full linter could not run, so I ran its fast mode, which passed. I did not run the full linter or the full suite.

## What the maintainer did

@slin1237 merged it. GitHub shows no maintainer comment, approval or change request. The only review thread was the one on the test, described above.

## The lesson

- A package can build and pass its own tests while the program no longer imports it. After a large change to an entry point, diff `go list -deps` for the entry point before and after, and read what dropped out.
- A manifest that declares a path is a promise the binary has to keep. Compare the two lists in a test. It names the next dropped registration too, not only this one.
- A guard test can pass for the wrong reason. Removing the registration already made mine fail, but it also matched a `Register` call on any receiver, so an unrelated call with the same path would have kept it green. Try to break the guard from more than one side.

If a manifest in your own project declares a path your binary never serves, reply on [X](https://x.com/spicysharkco).
