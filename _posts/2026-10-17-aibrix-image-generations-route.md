---
layout: post
title: "Two route lists, one missing path: image generations in AIBrix"
date: 2026-10-17
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3132
status: merged
tags: [model-serving, routing, gateway]
---

AIBrix's gateway accepted `POST /v1/images/generations`, but with no routing strategy set, the route it handed the request to had no match for that path, for every engine except `xdit`. I fixed it by adding one line to a list ([PR #3132](https://github.com/vllm-project/aibrix/pull/3132), merged 2026-10-17, about 55 minutes after I opened it). There was no existing issue for it.

## When it happens

The image guide in the repo (`docs/source/features/gateway-routing/omni-image.rst`) shows a `curl` call with a `model` field and no `routing-strategy` header. On a setup where nothing else sets a strategy, that call reached the gateway, and then the model's own route had nothing that matched the path. [PR #3017](https://github.com/vllm-project/aibrix/pull/3017) says that entries added to `modelPaths` "404 for models deployed before an upgrade". That has a different cause, a stale route, but the same shape: the model's route lacks the path. My evidence is reading the code and the unit test below, not a request through a running gateway, so I give no status code of my own.

## How a request travels

Without a routing strategy, a request passes through two routes: a shared one, then one per model. The shared route, `aibrix-reserved-router`, lists the API paths the gateway serves and sends them to the gateway plugin. That list lives in two files, `config/gateway/gateway-plugin/gateway-plugin.yaml` and the Helm chart's `httproute.yaml`. Both already had `/v1/images/generations`.

The plugin reads `model` from the body and looks for a routing strategy, in this order: one locked in the model's config, the `routing-strategy` request header, the config profile's strategy, then the `ROUTING_ALGORITHM` environment variable. If none is set, the strategy is `RouterNotSet`. The plugin then picks no pod. It sets a `model` header, and a code comment says `RouterNotSet` "delegates routing to the HTTPRoute/k8s Service".

That second route exists once per model. `buildHTTPRoute` in `pkg/controller/modelrouter/modelrouter_controller.go` builds it with one match per entry of `modelPaths`, and each match also requires the `model` header to equal the model's name. A path that is not in `modelPaths` has no match.

One engine was unaffected. For pods whose engine is `xdit`, `getEngineBasedPathRewrite` turns `/v1/images/generations` into `/generate`, and `/generate` was already in `modelPaths`. An engine that keeps the OpenAI path, such as vLLM-Omni, got no match.

## Why one list lacked the path

- [#1867](https://github.com/vllm-project/aibrix/pull/1867) (merged 2026-01-05) corrected the shared route from `/v1/image/generations` to the OpenAI spelling `/v1/images/generations`. Before it, the plugin rewrote the old path to `/generate` for every engine, and `/generate` was in `modelPaths`; it limited that rewrite to `xdit` pods. It changed nothing under `pkg/controller/modelrouter`. At that commit `modelPaths` had eight entries and none was an OpenAI-style image path.
- [#3086](https://github.com/vllm-project/aibrix/pull/3086) (merged 2026-10-14) added `/v1/images/edits` to both shared-route files and to `modelPaths`, "so it is served both with and without a routing strategy". It opened a `// Image` group in `modelPaths` holding only that path.

So before this PR, `/v1/images/edits` was in the shared route (both files) and in `modelPaths`, and `/v1/images/generations` was only in the shared route.

## The fix

```diff
 	// Image
+	"/v1/images/generations",
 	"/v1/images/edits",
```

A comment above `modelPaths` says it also sets the order of each route's matches and that new paths go into their group, so the line sits next to the edits path. Routes of models that already exist are brought up to date by the resync from #3017, so on a build that has both changes, existing models should pick up the new path after an upgrade.

## The test

```go
func TestBuildHTTPRouteMatchesImageGenerations(t *testing.T) {
	route := buildHTTPRoute("default", "demo", nil, nil)
	paths := routeMatchPaths(route)
	if !slices.Contains(paths, "/v1/images/generations") {
		t.Errorf("per-model HTTPRoute does not match /v1/images/generations, got %v", paths)
	}
}
```

The package already had tests that build a route and compare its match paths to `modelPaths`, among them `TestInformerEventsCreateHTTPRoute` and `TestEnsureHTTPRoutesUpdatesRouteMissingNewPaths`. The route and the expected value come from the same list, so a path missing from the list is missing from both sides and the comparison passes. The new test writes the path as a literal.

I checked that locally on the merge commit. With my line removed, 22 of the package's 23 tests passed, the existing comparison tests among them, and the new one failed with `per-model HTTPRoute does not match /v1/images/generations, got [...]`. With the line back, all 23 passed. I counted top-level tests in `go test -v` output, since plain `go test` prints no counts.

## What the maintainer did

A review bot posted a summary 30 seconds after I opened the PR, with nothing to change. @varungup90, who had also merged #3086 and #3017, approved with no review text and merged 13 seconds later. The PR has no other comments and stayed one commit.

## The lesson

- When an entry has to appear in two lists, adding it to one is half a change. Search for a neighbouring entry and compare where each one shows up: here the edits path was in both lists and the generations path in one.
- A test that takes its expected value from the code it checks cannot see an omission. Write the literal.
- Remove the fix and run the suite. If only your new test fails, the old ones were never checking that.

If a list in your own code has two homes and one of them drifted, reply on [X](https://x.com/spicysharkco).
