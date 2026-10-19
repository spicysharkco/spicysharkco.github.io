---
layout: post
title: "A start-tag search over the whole reply: non-stream reasoning in Xinference"
date: 2026-10-19
project: Xinference
pr: https://github.com/xorbitsai/inference/pull/5751
status: merged
tags: [model-serving, reasoning]
---

A non-stream chat reply that had no `<think>` before `</think>` but mentioned one after it either failed with an HTTP 500 or came back with the wrong `reasoning_content`. I fixed it in [PR #5751](https://github.com/xorbitsai/inference/pull/5751), approved about three hours and merged about five hours after I opened it.

## When it happens

`reasoning_content` is on and the chat template ends the prompt with `<think>`, as QwQ-32B's does. The reply has no `<think>` before `</think>`. The answer, or an argument of a tool call, mentions `<think>`:

```
reply:   "a</think>Use the <think> tag"
before:  IndexError, so the client got HTTP 500
after:   reasoning_content "a"   content "Use the <think> tag"

reply:   "a</think>b<think>c</think>d"
before:  reasoning_content "c"   content "b<think>c</think>d"
after:   reasoning_content "a"   content "b<think>c</think>d"
```

A reply with `<think>` before `</think>` is unchanged. I read the template and the request handler and did not run the model.

## How the reply is parsed

`prepare_reasoning_content` and `extract_reasoning_content` in `reasoning_parser.py` both put a `<think>` in front when the reply has none. A regex takes the text between the tags as `reasoning_content`, and the text after the first `</think>` is `content`. On the vLLM engine `_to_chat_completion` calls them, and `_post_process_completion` calls the second when the request has `tools`.

## Why the gap existed

The test for "no `<think>`" was `reasoning_start_tag not in model_output`, a search of the whole reply. That line is unchanged since the parser was added in February 2025. A `<think>` in the answer counted as present, so none was put in front. The regex then found no match, giving the `IndexError`, or matched the block in the answer, giving `c`.

## The fix

Search only the text before the first `</think>`. The whole change to `reasoning_parser.py`, trimmed to the changed lines:

```diff
@@ -276 +276,3 @@ class ReasoningParser:
-            if self.reasoning_start_tag not in model_output:
+            # A start token after the end token is part of the content.
+            before_end = model_output.split(self.reasoning_end_tag, 1)[0]
+            if self.reasoning_start_tag not in before_end:
```

## The test

`test_to_chat_completion_text_before_start_tag` in `test_utils.py` had two rows, with a newline before the start tag. I added a `reasoning` column and the two replies above, so each row names the `reasoning_content` it expects from `_to_chat_completion`.

Without the fix the two new rows fail and the two old rows pass. With it all four pass, as do the neighbouring reasoning tests. No new row covers `_post_process_completion`.

## What the maintainer did

@qinxuye approved with “LGTM”. It was merged with my diff unchanged. It was my fifth merged Xinference PR.

## The lesson

- Search for a marker only where it can mean something. `not in model_output` asked whether `<think>` appears anywhere. The code needed to know whether one comes before `</think>`.
- One cause had two shapes: one reply raised, the other returned `c` without an error. Compare values, not only whether the call raises.

If a check in your own code searches a whole input for a marker that only counts before another marker, reply on [X](https://x.com/spicysharkco).
