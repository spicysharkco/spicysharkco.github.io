---
layout: post
title: "Checking the new chunk first: a repeated end tag in streamed replies in Xinference"
date: 2026-10-19
project: Xinference
pr: https://github.com/xorbitsai/inference/pull/5756
status: merged
tags: [model-serving, reasoning, streaming]
---

Once a streamed reply's reasoning had ended, a later `</think>` in the answer was read as the end of the reasoning again. [PR #5756](https://github.com/xorbitsai/inference/pull/5756) fixed it, merged about five hours after I opened it.

## When it happens

A streaming request to QwQ-32B on the vLLM engine, with `reasoning_content` on. The template ends the prompt with `<think>\n`, so the converter puts that tag in front of the stream. The answer holds a second `</think>`. I read the templates and did not run the models.

```
chunks:  "a" "</think>" "b" "</think>" "c"
before:  reasoning_content "\na"   content "bc"
after:   reasoning_content "\na"   content "b</think>c"

chunks:  "a</think>b" "c</think>d"
before:  reasoning_content "\nac"  content "bd"
after:   reasoning_content "\na"   content "bc</think>d"
```

On qwen3.5, with thinking on and `reasoning_content` set at launch, not in the request, no tag is inserted and a `<think>` chunk in the answer was dropped: `use `, `<think>`, `x` gave `use x`.

## How the parser decides

`extract_reasoning_content_streaming` in `reasoning_parser.py` runs once per chunk, given the text so far and the new chunk. Two arms asked whether `</think>` was in the text so far, but each came after tests on the new chunk. A chunk with `</think>`, or with `<think>` when no start tag had streamed, never got past those tests.

## Why the gap existed

The order dates from the parser's first version, in February 2025. The table I extended had no row with a second `</think>`.

## The fix

Return the chunk as content once `</think>` is in the text so far. Both old arms test that, so the diff deletes them: the `elif self.reasoning_end_tag in previous_text:` arm in two branches, 12 lines. Shown: the added block, without its two comment lines.

```diff
@@ -128,0 +129,7 @@ class ReasoningParser:
+        if self.reasoning_end_tag in previous_text:
+            delta["reasoning_content"] = None
+            delta["content"] = delta_text
+            return self._log_streaming_result(previous_text, delta_text, delta)
+
```

## The test

`test_to_chat_completion_chunks_start_tag_after_leading_whitespace` runs both chunk formats through the sync and async converters. I added an `insert_start_tag` column and four rows: the three examples above, and the first again on qwen3.5. Without the fix the new rows fail in both formats, 8 of 12 cases; with it all 12 pass.

No row can fail the deleted arms. Over every sequence of up to five pieces from a small set, for three tag pairs (about 520,000 sequences), the code with only the early return and the merged code gave the same deltas.

## What the maintainer did

@qinxuye approved with “LGTM”. It was merged eight minutes later with my diff unchanged. It was my sixth merged Xinference PR.

## The lesson

- Ask whether the stream is already past the marker before you look at the new chunk.
- A dead branch cannot fail a test. Compare outputs with and without it over every short input before deleting it.

If a parser in your own code tests the new input before the state it has already seen, reply on [X](https://x.com/spicysharkco).
