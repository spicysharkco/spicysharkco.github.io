---
layout: post
title: "Counter rules keyed with a suffix the parser drops: runtime metrics in AIBrix"
date: 2026-10-18
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3147
status: merged
tags: [model-serving, prometheus, testing]
---

The runtime sidecar in AIBrix, a control plane for serving LLMs on Kubernetes, copies engine metrics to `aibrix:` names. Three documented counters never appeared: `aibrix:prompt_tokens_total`, `aibrix:generation_tokens_total` and `aibrix:request_success_total`. I fixed it in [PR #3147](https://github.com/vllm-project/aibrix/pull/3147), merged about seventeen hours after I opened it.

## When it happens

With the defaults, `INFERENCE_ENGINE=vllm` and transformation on, `/metrics` served the `vllm:` counters and none of the three copies. The copy was skipped silently.

## How the copy works

`HTTPCollector` looks up each parsed metric family in the rules by `m.name` (`http_collector.py:86`). The parser, `prometheus_client` 0.20 as pinned, drops `_total` from a counter's family name. For `vllm:prompt_tokens_total` it returns the family `vllm:prompt_tokens` with the sample `vllm:prompt_tokens_total`. `RenameStandardRule` names each sample as the new name plus whatever follows the old name in the sample name.

## Why the gap existed

Five of the six `_total` keys date from #1551 (September 2025); the TRT-LLM key came in May 2026. The tests that came with them check each key and new name in the table. None runs a counter through the collector with these rules.

## The fix

Key the six rules by the family name and drop `_total` from each new name. Samples keep their suffix, so the served names are the documented ones. The diff is trimmed to one of the six rules, with no context lines. The other five change alike, and the commit adds a two-line comment.

```diff
@@ -36,2 +39,2 @@ VLLM_METRIC_STANDARD_RULES: Dict[str, StandardRule] = {
-    "vllm:prompt_tokens_total": RenameStandardRule(
-        "vllm:prompt_tokens_total", "aibrix:prompt_tokens_total"
+    "vllm:prompt_tokens": RenameStandardRule(
+        "vllm:prompt_tokens", "aibrix:prompt_tokens"
```

## The test

`test_collect_engine_counters_are_standardized` has six rows: three vLLM, two SGLang, one TRT-LLM. Each feeds a `# TYPE <name> counter` line through `HTTPCollector` with that engine's rules and expects the `aibrix:` sample. Seven older tests of the rule table now expect family names.

With the fix reverted and the tests kept, 13 tests fail in `tests/metrics`. With the fix, all 46 pass. Reverting one rule fails only its row of that test.

## What the maintainer did

@varungup90 reviewed it about an hour and twenty minutes after I opened it. They merged it as a squash about fifteen and a half hours later, with no change to the code.

## The lesson

- Print the key from the real parser before you write the table. A counter's family name has no `_total`, and a lookup that skips a miss stays silent.
- Check the names your docs list against the output. The runtime docs listed three `aibrix:` counters and the output held none. Seven rule-table tests passed on the base because they asserted the same keys.
- Change every name that feeds the rename. A sample name is the new name plus the text after the old name, so a new name that kept `_total` would end in `_total_total`.

If a rule table in your own code is keyed by a name from another system, reply on [X](https://x.com/spicysharkco).
