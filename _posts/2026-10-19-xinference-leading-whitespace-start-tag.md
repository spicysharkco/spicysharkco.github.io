---
layout: post
title: "A start tag inserted ahead of a newline: streamed reasoning in Xinference"
date: 2026-10-19
project: Xinference
pr: https://github.com/xorbitsai/inference/pull/5746
status: merged
tags: [model-serving, reasoning, streaming]
---

A streamed reply that starts with a lone newline chunk, with `reasoning_content` on, put the model's own `<think>` inside `reasoning_content`. I fixed it in [PR #5746](https://github.com/xorbitsai/inference/pull/5746), merged about three and a half hours after I opened it.

## When it happens

A streaming request to GLM-4.5 or GLM-4.6 on the vLLM engine, with `reasoning_content` on, when the model streams its `"\n"` as a delta of its own. I read the templates and did not run the models.

For the chunks `["\n", "<think>", "a", "</think>", "b"]` the client got:

```
before:  reasoning_content "\n\n<think>a"   content "b"
after:   reasoning_content "a"              content "b"
```

## How the helper decides

`prepare_reasoning_content_streaming` and `prepare_reasoning_content_sync` in `reasoning_parser.py` look at the first chunk with text. With no start tag in it, they put a `<think>\n` chunk in front. The stream parser reads all text up to the end tag as `reasoning_content`, so a later `<think>` is reasoning text.

## Why the gap existed

Only chunks with no text are passed over. `"\n"` has text and no tag, so the tag went in before it.

## The fix

Hold whitespace-only text back: its chunk goes out with empty text and the decision waits for the next chunk with more than whitespace. If that chunk has the tag, the whitespace is dropped, as the non-stream path drops text before the tag. If not, it goes back in after the inserted `<think>\n`. A reply of only whitespace now streams as empty.

Trimmed: the chat-format hunks of `prepare_reasoning_content_streaming`, no context lines. The completion format and `prepare_reasoning_content_sync` repeat them.

```diff
@@ -422,0 +423 @@ class ReasoningParser:
+        leading_space = ""
@@ -445,0 +447,7 @@ class ReasoningParser:
+                    if text.isspace():
+                        # Whitespace can come before the start tag, so hold it
+                        # back until a chunk shows whether the tag follows.
+                        leading_space += text
+                        delta["content"] = ""
+                        yield chunk
+                        continue
@@ -454 +462 @@ class ReasoningParser:
-                            chunk, f"{self.reasoning_start_tag}\n"
+                            chunk, f"{self.reasoning_start_tag}\n{leading_space}"
```

## The test

`test_to_chat_completion_chunks_start_tag_after_leading_whitespace` in `test_utils.py` runs the sync and async chat converters on the stream above and on `["\n", "hi"]`, in both chunk formats. It joins `reasoning_content` and `content` over the deltas and expects `"a"` and `"b"`, then `"\n\nhi"` and `""`.

Without the fix 2 of the 4 cases fail, the ones for the first stream; with it all 4 pass. Reverting any one of the ten changed places fails a case.

## What the maintainer did

@qinxuye approved with “LGTM” and merged it. It was my fourth merged xinference PR.

## The lesson

- "The first chunk with text" need not be the first chunk that can decide. Hold back what says nothing yet, such as a lone newline.
- Join the deltas in a stream test: the fix moves whitespace between chunks, and the joined fields are what a client assembles. `["\n", "hi"]` passes before the fix but alone reaches the four lines that add the whitespace back.

If a stream reader in your own code decides on its first chunk with text, reply on [X](https://x.com/spicysharkco).
