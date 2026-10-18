---
layout: post
title: "I made xinference drop the start tag from a reasoning reply that never ended"
date: 2026-10-18
---

A non-stream chat reply that stopped inside the thinking block, for example at `max_tokens`, came back with `<think>` at the front of `reasoning_content`. I fixed it in [PR #5731](https://github.com/xorbitsai/inference/pull/5731), merged about five hours after I opened it. It follows [my earlier xinference fix](https://spicyshark.co/2026/10/17/xinference-async-reasoning-leading-chunks.html).

## When it happens

A reasoning model runs with `reasoning_content` on, and a non-stream request gets a reply with `finish_reason: "length"` and no end tag. The start tag is there whether the model wrote `<think>` itself or Xinference put it in front. The reply came back as:

```
reasoning_content: "<think>\nThe user asks"
content: ""
```

It now gives `"\nThe user asks"`, as the same request does streamed. Streams and replies that finish thinking were not affected.

## How the reply is parsed

`_to_chat_completion` in `utils.py` calls `prepare_reasoning_content`, which puts `<think>\n` in front of the text when the parser inserts the start tag and the text has none. Then it calls `extract_reasoning_content` in `reasoning_parser.py`. With an end tag, that method takes the text between the tags. Without one, it returned the whole text.

## Why the gap existed

A comment above that branch says DeepSeek-R1 does not write the start tag, so the text is taken as reasoning from its start. That assumes the text has no start tag, yet `_to_chat_completion` puts one in first. The streaming parser and the Kimi-K3 parser in the same file already cut at the start tag.

## The fix

When there is no end tag and the text has a start tag, keep what follows the first one. The hunk in `reasoning_parser.py`, with one line of context on each side:

```diff
@@ -265,2 +265,4 @@ class ReasoningParser:
         if self.reasoning_end_tag not in model_output:
+            if self.reasoning_start_tag in model_output:
+                model_output = model_output.split(self.reasoning_start_tag, 1)[1]
             logger.debug(
```

## The test

`test_to_chat_completion_stopped_while_thinking` in `test_utils.py` gives `_to_chat_completion` the text `"The user asks"`, where Xinference adds the start tag, and the text `"<think>\nThe user asks"`, where the model wrote it. Each is a `length` reply with no end tag, and the test expects `content == ""`, `finish_reason == "length"` and `reasoning_content == "\nThe user asks"`.

Before the fix both cases failed, with `"<think>\nThe user asks"` in place of `"\nThe user asks"`. After it both passed, as did 38 neighbouring cases. Removing the two source lines failed both again.

## What the maintainer did

@qinxuye approved with “LGTM” and merged it. It was my second xinference PR.

## The lesson

- When a parser has a branch for "end seen" and one for "end missing", check that the second cleans up as much as the first.
- Take the expected value from the path that already works. The stream gave `"\nThe user asks"` for this input, so the non-stream test asserts the same string.
- When one value can arrive from two sources, test each. Xinference added the start tag in one case and the model wrote it in the other; a fix that stripped only the added tag would pass the first and fail the second.

If a parser in your own code has a branch for a reply that never finished, reply on [X](https://x.com/spicysharkco).
