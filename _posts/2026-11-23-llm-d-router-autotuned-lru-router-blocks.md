---
layout: post
title: "An LRU sized in the wrong block: autotuned capacity in the llm-d-router prefix index"
date: 2026-11-23
project: llm-d-router
pr: https://github.com/llm-d/llm-d-router/pull/3476
status: merged
tags: [model-serving, go, caching]
---

The approximate prefix cache producer in llm-d-router (`approx-prefix-cache-producer`) keeps one LRU of block hashes per pod, as an estimate of what each model server still has cached. With `autoTune` on, it sized that LRU from the engine's block count. But each LRU entry is one router block, and a router block can be larger than an engine block. With vLLM's default block size of 16, the LRU could cover four times as many tokens as the engine's KV cache, and up to 64 times with a block size of 1. I sent the fix ([PR #3476](https://github.com/llm-d/llm-d-router/pull/3476), merged 2026-10-20). It fixes [#3139](https://github.com/llm-d/llm-d-router/issues/3139), where @JamieLee0510 had written up the analysis and a reproduction.

## When it happens

The router block size comes from `GetBlockSize`: the engine's `prefix_match_unit` if it reports one, else the engine's block size, and never less than `minBlockSizeTokens`, which is 64. With `autoTune` on, `makeserver` took each pod's LRU capacity straight from `CacheNumBlocks`, the engine's block count.

Take an engine that reports 31250 blocks of 16 tokens, the first case in the test below. It holds 500000 tokens. The router block is 64 tokens, so 31250 LRU entries cover 2000000 tokens. The LRU can then keep prefixes the engine has already evicted, and consumers of the prefix match, such as `prefix-cache-scorer`, can overestimate cache hits.

## Why the gap existed

`GetBlockSize`'s comment gives the reason for the floor: the indexer holds one LRU entry per pod and block, so small blocks inflate its memory. `makeserver` did not call `GetBlockSize`; it used the engine's count as it was. That was fine while the engine reported no `prefix_match_unit` and its block was at least 64 tokens, because the two block sizes were then equal.

The only test in the package that set `CacheNumBlocks` before this PR, `TestPrefixPluginAutoTune`, reports `CacheBlockSize: 128`. Its comment says the value is above `minBlockSizeTokens` so that the metric passes through unclamped. That test checks only that the pod is tracked, so no test checked the capacity `makeserver` computes.

## The fix

```diff
 		gpuBlocks = targetEndpoint.GetMetrics().CacheNumBlocks
+		// CacheNumBlocks counts engine blocks, but each LRU entry is one router
+		// block, which minBlockSizeTokens can make larger. Size the LRU to the
+		// engine's token capacity so it does not outlive the engine's cache.
+		if engineBlockSize := targetEndpoint.GetMetrics().CacheBlockSize; engineBlockSize > 0 {
+			gpuBlocks = max(1, gpuBlocks*engineBlockSize/p.GetBlockSize([]fwksched.Endpoint{targetEndpoint}))
+		}
 	} else if p.config.LRUCapacityPerServer > 0 {
```

The capacity is now `CacheNumBlocks * CacheBlockSize / GetBlockSize()`, rounded down and at least 1.

- `Produce` also calls `GetBlockSize` to cut a prompt into blocks, so the capacity is converted with the same function that cuts prompts into blocks. Each call reads one endpoint's metrics, so they agree when the pods report the same block size.
- The multiplication comes first, so integer division rounds once, at the end.
- `max(1, ...)` matters because the indexer treats `NumOfGPUBlocks <= 0` as a request for the default size, the configured `lruCapacityPerServer`. A zero would give a tiny cache the default capacity.
- The `engineBlockSize > 0` guard leaves the capacity as it was when the engine does not report its block size.

When the engine reports `prefix_match_unit`, the router block follows it, still floored at 64, and the capacity is converted the same way. In the last row of the table below, the router block (64) is smaller than the engine block (4096), so the capacity goes up from 1000 to 64000 entries. Before, that LRU covered 64 times fewer tokens than the engine's cache. The capacity is unchanged when the router block equals the engine block, or when the engine reports no block size.

## The test

`TestMakeserver_AutotuneCapacityInRouterBlocks` is a table test with six cases. Each builds a data producer with `newDataProducer` and `AutoTune: true`, gives it an endpoint with the metrics from the row, and checks the capacity `makeserver` hands to the indexer:

```go
assert.Equal(t, tc.want, p.makeserver(endpoint).NumOfGPUBlocks)
```

| Case | Engine block | Prefix match unit | Engine blocks | Router block | LRU entries |
|---|---|---|---|---|---|
| below the floor | 16 | none | 31250 | 64 | 7812 |
| at the floor | 64 | none | 1000 | 64 | 1000 |
| above the floor | 128 | none | 1000 | 128 | 1000 |
| smaller than one router block | 16 | none | 2 | 64 | 1 |
| block size not reported | none | none | 1000 | n/a (`makeserver` skips `GetBlockSize`) | 1000 |
| prefix match unit below the engine block | 4096 | 32 | 1000 | 64 | 64000 |

With `plugin.go` from the parent commit and the new test, three cases fail (assertion lines, whitespace trimmed):

```
expected: 7812
actual  : 31250
expected: 1
actual  : 2
expected: 64000
actual  : 1000
```

The other three pass either way. They pin the cases that must not move. At the merge commit all six pass.

## What it does not change

`lruCapacityPerServer` keeps its default of 31250. It still applies without `autoTune`, or when the engine reports no block count. The comment on its default gets 31250 by dividing 500K tokens by a block size of 16, and an entry is at least 64 tokens, so on those paths the default and the entry size still do not match. That needs a separate decision, and the PR leaves it.

When the engine reports its block size, rounding down keeps the LRU at or below the engine's token count, except where the minimum of one entry applies: two blocks of 16 tokens are 32 tokens, and one router block is 64. I did not measure routing or hit rates on a running deployment. The evidence in the PR is the table test.

The PR had no inline review comments. @blacksoxx approved it on 2026-10-16, and @liu-cong approved and merged it on 2026-10-20.

## The lesson

- When a size is counted in one unit and consumed in another, convert it through the function that defines the unit. `makeserver` now asks `GetBlockSize` instead of repeating its rules.
- A test above a clamp cannot see the clamp. Put rows below, at and above the boundary, and one where the input is missing.
- After integer division, check zero. Here a zero capacity would have meant "use the default".

If an index of yours is sized in one block size and filled in another, reply on [X](https://x.com/spicysharkco).
