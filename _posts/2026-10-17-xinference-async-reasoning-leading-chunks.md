---
layout: post
title: "I made xinference keep the chunks of an empty reasoning reply"
date: 2026-10-17
---

A streamed chat reply with no text at all, from a reasoning model that needs the `<think>` tag added, reached the client as `data: [DONE]` and nothing else. No `finish_reason`, no usage chunk. I fixed it in [PR #5726](https://github.com/xorbitsai/inference/pull/5726), approved within half an hour and merged about two hours after I opened it.

## When it happens

The reply has no text, for example because it stops on its first token. The engine streams a chunk with `text: ""` and `finish_reason: "stop"`, and with `stream_options: {"include_usage": true}` a usage chunk with `choices: []`.

The reasoning helper dropped every one of them. The chat converter built on it produced no chunks, and the stream body held only `[DONE]`. After the fix the same input gives the stop chunk and the usage chunk.

## How the helper waits

`prepare_reasoning_content_streaming` in `reasoning_parser.py` waits for the first chunk with text, then puts a chunk with the start tag in front of it. While it waits, four early exits skip a chunk with `continue`: no `choices[0]`, a `None` delta, an empty `delta.content`, an empty `text`. None of them passed the skipped chunk on.

An ordinary chat reply lost only its empty role chunk, and the role moved to the next one, so nothing visible was lost. With the fix that chunk arrives first, ahead of the start tag chunk, as it already does from the sync helper.

## Why the gap existed

[#5716](https://github.com/xorbitsai/inference/pull/5716) had just added the same yields to `prepare_reasoning_content_sync`. The async helper kept the old code.

## The fix

Yield the chunk before each `continue`. The hunks from `reasoning_parser.py`:

```diff
@@ -427,2 +427,3 @@ class ReasoningParser:
                 if not choices or not choices[0]:
+                    yield chunk
                     continue
@@ -435,2 +436,3 @@ class ReasoningParser:
                     if delta is None:
+                        yield chunk
                         continue
@@ -442,2 +444,3 @@ class ReasoningParser:
                         )
+                        yield chunk
                         continue
@@ -460,2 +463,3 @@ class ReasoningParser:
                         )
+                        yield chunk
                         continue
```

The yield at the `None` delta exit is only there to match the sync helper. No engine in the repo sends that shape.

## The test

Both tests are in `test_utils.py` and need no model or network. `test_async_prepare_reasoning_keeps_leading_chunk_without_text` feeds the helper a leading chunk of each of the four shapes, then a text chunk. It expects the leading chunk, the start tag chunk and the text chunk, in that order. `test_async_to_chat_completion_chunks_with_reasoning_keeps_empty_stream` runs a stop chunk with empty text and a usage chunk through `_async_to_chat_completion_chunks`. It expects the `finish_reason` and the usage back.

The first test runs once per shape, so the two tests make 5 cases. Before the fix all 5 failed. After it all 5 passed, along with 65 neighbouring cases. Each of the four shapes reaches a different exit, so every yield has a case that needs it.

## What the maintainer did

@qinxuye approved with “LGTM” and merged it. It was my first xinference PR.

## The lesson

- When a fix lands in one half of a sync and async pair, read the other half.
- A chunk with no text can still carry the `finish_reason` or the usage. A filter that waits for text must pass on what it skips.
- Test the empty case. A stream whose first chunk has text never reaches the early exits, so tests built on one pass with or without the fix.

If a filter in your own code skips chunks while it waits for the first good one, reply on [X](https://x.com/spicysharkco).
