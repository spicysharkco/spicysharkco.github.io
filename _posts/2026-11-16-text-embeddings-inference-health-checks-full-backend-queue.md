---
layout: post
title: "Health checks in a full backend channel: embed and predict sends in Text Embeddings Inference"
date: 2026-11-16
project: Text Embeddings Inference
pr: https://github.com/huggingface/text-embeddings-inference/pull/952
status: merged
tags: [model-serving, rust, concurrency]
---

Text Embeddings Inference (TEI) could exit in the middle of serving traffic. Concurrent `/health` calls during a long forward pass filled the channel that feeds the backend thread. The batching task's next `embed` call then met the full channel and panicked with `Full(..)`, and the release build aborts on panic. I sent the fix ([PR #952](https://github.com/huggingface/text-embeddings-inference/pull/952), merged 2026-11-03). It closed [#424](https://github.com/huggingface/text-embeddings-inference/issues/424); [#857](https://github.com/huggingface/text-embeddings-inference/issues/857) reports the same panic and was still open on 2026-11-16.

## When it happens

TEI runs the model on one backend thread, which takes one command at a time from a channel with 8 slots. Health checks, embed calls and predict calls all share those slots. When the backend is healthy, `Backend::health` queues a `Health` command with `send(..).await`: each concurrent `/health` request waits for a slot, then holds it until the backend thread reaches it. During a long forward pass, enough health checks fill all 8 slots.

`Backend::embed` and `Backend::predict` did not wait. They used `try_send(..).expect(..)`, so a full channel was a panic:

```
thread 'tokio-rt-worker' panicked at backends/src/lib.rs:423:14:
No backend receiver. This is a bug.: "Full(..)"
```

The release profile sets `panic = "abort"`, so that panic ended the process. The usual caller is the batching task, so a regular request hit it.

I reproduced it on a CPU build with `sentence-transformers/all-MiniLM-L6-v2` and `RAYON_NUM_THREADS=4`, to make forward passes slow. A standard-library Python script sent batches of 32 inputs from 4 embed threads and polled `/health` from 32 threads, for 75 seconds. Without the fix the script reported the server gone at 11.0 s, with the panic above in the server log. As a control, the same embed load without health traffic ran 40 s with no panic. So the health traffic filled the channel.

## Why the gap existed

The message says the receiver is gone, but the panics quoted in #424 and #857 carry `Full(..)`: the backend thread was alive and behind. @snowch pointed out that difference in [#424](https://github.com/huggingface/text-embeddings-inference/issues/424#issuecomment-5396056653). They also traced a second path to the same line: while the backend is flagged unhealthy, `health()` checks it with a one-token `embed` or `predict`, which goes through the same `try_send`. Their reproduction needed memory pressure to flip that flag. The one above stays on the healthy branch.

## The fix

In `embed` and in `predict`, one line changes and `.await` is added:

```diff
         self.backend_sender
-            .try_send(BackendCommand::Embed(batch, Span::current(), sender))
+            .send(BackendCommand::Embed(batch, Span::current(), sender))
+            .await
             .expect("No backend receiver. This is a bug.");
```

That is what `health` already did for its own command. A full channel now makes the caller wait instead of ending the process. The `expect` stays, and fires only when the channel is closed, as in `health`.

@snowch had proposed mapping `Full` to an overload error and a 429. The PR does not, for three reasons. The `Full` comes from health commands holding slots in the channel. The only hot-path caller is the batching task, so an error there would fail a whole batch that the front door had already admitted. And TEI already returns 429 at the front door.

With the fix, the same 75 s run finished with the server up: 9 embed requests and 2334 health checks, all 200, and no panic in the log.

## The test

`embed_and_predict_wait_when_backend_channel_is_full` uses a mock backend whose `embed` blocks until the test releases it, with the real backend thread and channel. A first `embed` holds the thread. The test then queues 16 health checks and waits until the channel has no capacity left. It starts one `embed` and one `predict`, and after 200 ms asserts that both are still pending:

```rust
assert!(!embed.is_finished(), "embed gave up on a full channel");
assert!(!predict.is_finished(), "predict gave up on a full channel");
```

Then it releases the backend and checks that every queued call completes. Without the fix it fails with the same `Full(..)` panic, at the `embed` and the `predict` send. With the fix it passes.

## What it does not change

`/health` still goes through the same queue. Under sustained load a probe can wait behind forward passes, and a short client timeout can still fail. Health commands also add latency to forward passes. There is still no timeout on the send: if the backend thread hangs while alive, `embed` and `predict` now wait for a slot, as `health` already did. The PR only stops a full queue from ending the server. Making `/health` independent of the queue would be a separate change.

The PR had no review comments. @alvarobartt merged it on 2026-11-03, and on 2026-11-06 added it to the v1.10.0 milestone.

## The lesson

- On a bounded channel whose consumer keeps draining it, `Full` is backpressure, not a bug. `try_send(..).expect(..)` turns it into a crash. Wait with `send(..).await`, unless the caller has a real way to report the error.
- A probe that shares a queue with real work uses its capacity. Load-test the probe endpoint together with the work, and run the same load without it as a control.
- Read the error value, not only the message. Here the message said the receiver was gone; the value said the queue was full.

If a `try_send` in your code can meet a full queue, reply on [X](https://x.com/spicysharkco).
