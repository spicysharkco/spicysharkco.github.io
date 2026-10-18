---
layout: post
title: "An end index counted from zero: content after a leading newline in Xinference"
date: 2026-10-18
project: Xinference
pr: https://github.com/xorbitsai/inference/pull/5733
status: merged
tags: [model-serving, reasoning]
---

A non-stream chat reply with a newline before the `<think>` tag came back with a stray `>` at the start of `content`. I fixed it in [PR #5733](https://github.com/xorbitsai/inference/pull/5733), approved within a quarter of an hour and merged about an hour and forty minutes after I opened it.

## When it happens

With `reasoning_content` on, a reasoning model's reply has text before the start tag:

```
reply:             "\n<think>\nThe user asks\n</think>\n\nHello"
reasoning_content: "\nThe user asks\n"
content:           ">\n\nHello"        (now "\n\nHello")
```

With nothing after the end tag, `content` was `">"` instead of `""`. Two characters before the tag leaked `"k>"`. Replies that start with the tag were not affected.

A newline before the tag may come from a chat template. vllm-project/vllm#23479 reports GLM-4.5-Air streaming a newline before its reasoning, which the reporter ties to its template. That is a streaming report and I did not run the model, so I cannot say a non-stream reply carries one.

## How the reply is cut

The path is the one in [my last xinference fix](https://spicyshark.co/2026/10/18/xinference-reasoning-start-tag-without-end-tag.html). `prepare_reasoning_content` leaves a reply that already has a start tag alone. Then `extract_reasoning_content` in `reasoning_parser.py` takes `reasoning_content` from a regex match and cuts `content` at `len(start + reasoning + end)`, the length of the block.

## Why the gap existed

That index has been there since the parser was added in February 2025 ([#2905](https://github.com/xorbitsai/inference/pull/2905)). Just above it the method adds a missing start tag at index 0, and a block at index 0 ends at its own length. A start tag after other text was not handled.

## The fix

Cut at the first end tag. The diff is trimmed to the changed lines of the one hunk in `reasoning_parser.py`.

```diff
@@ -281,4 +281 @@ class ReasoningParser:
-            end_index = len(
-                f"{self.reasoning_start_tag}{reasoning_content}{self.reasoning_end_tag}"
-            )
-            final_output = model_output[end_index:]
+            final_output = model_output.split(self.reasoning_end_tag, 1)[1]
```

`_post_process_completion` also calls it, for models with a tool parser, and gets the same fix.

## The test

`test_to_chat_completion_text_before_start_tag` in `test_utils.py` gives `_to_chat_completion` the reply above, once with `"\n\nHello"` after the end tag and once with nothing. It expects `reasoning_content == "\nThe user asks\n"` and a `content` of `"\n\nHello"` or `""`.

Before the fix both cases failed; after it both passed, in each of three runs. The whole `test_utils.py` module passed (97), and so did 23 reasoning tests in other files.

## What the maintainer did

@qinxuye approved with “LGTM” and merged it with my diff unchanged. It was my third xinference PR.

## The lesson

- Cut at the delimiter, not at a length you add up from the pieces.
- A stray piece of a delimiter at the edge of a field, here `>`, means an index is off by the distance the block moved.

If your own parser finds the end of a block by adding up lengths, reply on [X](https://x.com/spicysharkco).
