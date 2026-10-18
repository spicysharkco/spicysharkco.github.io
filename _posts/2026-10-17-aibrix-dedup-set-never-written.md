---
layout: post
title: "The duplicate guard that never recorded a path"
date: 2026-10-17
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3133
status: merged
tags: [model-serving, routing, testing]
---

I found a set under `// avoid duplicates` that nothing wrote to. AIBrix's `appendCustomModelRouterPaths` checked it before appending a path, so repeated annotation entries became repeated HTTPRoute matches. I fixed it in [PR #3133](https://github.com/vllm-project/aibrix/pull/3133), merged 2026-10-17 about 1 hour 57 minutes after I opened it. The impact was narrow: identical matches did not change routing, but they used space near the 64-match limit.

## When it happens

The input is the custom-path annotation defined by [`ModelAnnoRouterCustomPath`](https://github.com/vllm-project/aibrix/blob/f06d171f9eb9cde301d894954599f27c4c1dcce5/pkg/constants/model.go#L119-L121). A list such as `/version,/score,/version` added the first path again at the end. The helper also removed spaces before checking for duplicates, so `/f oo` and `/foo` became the same path and still both got appended. A custom entry equal to a default path could repeat a match too.

I do not have an outage report. My evidence is the code, the failing table-test row, and the local count-and-schema check recorded in the PR.

## How the paths reach a route

In `pkg/controller/modelrouter/modelrouter_controller.go`, workload annotations reach `createHTTPRoute`. `buildHTTPRoute` first makes the default matches from `modelPaths`, then calls `appendCustomModelRouterPaths` to extend the route.

The helper reads the comma-separated annotation, removes spaces, and skips empty entries. Each accepted custom path becomes a prefix match with the same model-header condition as the default matches. All of these matches go into the same rule. Repeating an identical path here adds no new routing choice.

There was already a `pathSet`, and the loop checked whether it contained each normalized path. But the set started empty and stayed empty. The `exists` check could never find an earlier entry. Seeing a set and a duplicate-check comment was not enough; I had to look for the write.

## Where the guard came from

I traced the helper to [#1841](https://github.com/vllm-project/aibrix/pull/1841), which added custom paths through an annotation. Its [first commit](https://github.com/vllm-project/aibrix/commit/e927436255bf48592e282c4a8be558f32d0fee44) used a `pathMap` and recorded accepted paths. A [follow-up in that PR](https://github.com/vllm-project/aibrix/commit/de5419074d12d5b3633166c7c366b77148ae25db) replaced it with `pathSet` and dropped the write. The empty-set guard was already present when #1841 merged.

My first fix filled that gap inside the loop. It kept the first occurrence and recorded each path before appending it. That solved repeated entries within the annotation. Review made me look at the matches already on the route as well.

## The fix

This is the relevant part of the merged change, with the existing skip check kept as context:

```diff
 pathSlice := strings.Split(paths, ",")
 // avoid duplicates
 pathSet := make(map[string]struct{})
+for _, match := range httpRoute.Spec.Rules[0].Matches {
+    if match.Path != nil && match.Path.Value != nil {
+        pathSet[*match.Path.Value] = struct{}{}
+    }
+}
 for _, path := range pathSlice {
     // remove illegal space in path
     path = strings.ReplaceAll(path, " ", "")
     if _, exists := pathSet[path]; path == "" || exists {
         continue
     }
+    pathSet[path] = struct{}{}
```

Seeding the set means a custom path already covered by a default match is skipped. Recording each new entry means later repeats in the annotation are skipped too. The first occurrence keeps its place; the change does not reorder the remaining paths.

## How small is the impact?

At this change, `modelPaths` had 22 entries. The HTTPRoute schema used by the pinned Envoy Gateway dependency allowed 64 matches per rule. That left room for 42 distinct custom paths outside the default set.

The PR records a local check against that schema: 42 such paths plus a repeat made 65 matches before the fix, so the route exceeded the limit. After deduplication, it had 64. Repeating a default path had the same effect. A list of 43 distinct non-default paths still made 65 matches after my change; removing duplicates cannot make that list fit.

On initial creation, a rejected route makes `createHTTPRoute` log `Failed to create httproute` and return. That is a concrete consequence, but it requires a long list near the cap. I would not describe this as a general routing fix. Most lists only lose useless repeated matches.

## The test

I added a row to `TestAppendCustomModelRouterPaths` in `pkg/controller/modelrouter/modelrouter_controller_test.go`. It starts with an `/origin` match and supplies `/foo,/bar,/foo, /f oo ,/bar,/origin`. The expected paths are `/origin`, `/foo`, `/bar`. The existing table checks also verify the model header on added matches.

The final PR body records the failure before the fix as `expected 3 matches, got 7`. Across the package, the verbose run showed 22 top-level tests passing and 1 failing. With the fix, all 23 passed. Those are the recorded local results, rather than a new run for this post.

## What happened in review

A review bot caught the limit of my first version: adding to an initially empty set only deduplicated the annotation itself. A custom entry could still duplicate a default match.

I added the initial scan in [f06d171f](https://github.com/vllm-project/aibrix/commit/f06d171f9eb9cde301d894954599f27c4c1dcce5) and extended the repeated-path test with `/origin`. That existing match is why the row now checks both kinds of repetition. @varungup90 merged the PR.

## The lesson

- When a guard reads a set, find every write. An empty set can make correct-looking control flow inert.
- Define the initial state before testing a loop. The route already had matches before custom paths arrived.
- Put a number on low impact. Here the cap and default count explain exactly when redundant entries matter.

If you have found a guard whose state never changed, reply on [X](https://x.com/spicysharkco).
