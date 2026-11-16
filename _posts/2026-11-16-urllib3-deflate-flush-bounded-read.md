---
layout: post
title: "A decoder flush skipped at the end of the body: HTTPResponse.read(amt) in urllib3"
date: 2026-11-16
project: urllib3
pr: https://github.com/urllib3/urllib3/pull/5316
status: merged
tags: [observability-data, python, http]
---

urllib3's `HTTPResponse.read(amt)` could return a shorter body than the server sent, with no error. A raw deflate body that decodes to 6000 bytes, read 64 bytes at a time, came back as 5952 bytes. I sent the fix ([PR #5316](https://github.com/urllib3/urllib3/pull/5316), merged 2026-11-11). A companion fix in the same code, [PR #5315](https://github.com/urllib3/urllib3/pull/5315), fixed a gzip read that returned the same member again on every call. Neither PR closes an issue.

## When it happens

```python
import io
import zlib

from urllib3.response import HTTPResponse

data = b"abc" * 2000
c = zlib.compressobj(wbits=-zlib.MAX_WBITS)
body = c.compress(data) + c.flush()

resp = HTTPResponse(
    io.BytesIO(body),
    headers={"content-encoding": "deflate"},
    preload_content=False,
)
out = b""
while chunk := resp.read(64):
    out += chunk
print(len(data), len(out))
```

Without the fix this prints `6000 5952`; with it, `6000 6000`. `resp.stream(64)` lost the same 48 bytes, and so did `resp.read(5952)` followed by `resp.read()`, both over `BytesIO` and over a real `http.client.HTTPResponse`. `resp.read1(64)` returned all 6000 bytes.

## Why the gap existed

Since the decompression-bomb safeguards in c19571d, urllib3 asks zlib for at most the bytes a read still needs. When a read stops at exactly `amt` bytes, zlib can still hold decoded output that only `flush()` returns. `Decompress.unconsumed_tail` reports only unread input, so urllib3's `has_unconsumed_tail` check did not see it. zlib and gzip bodies end in a checksum that is still unread at that point. A raw deflate body has no trailer. I ran that 6000-byte payload as complete zlib, gzip, zstd and raw deflate bodies through `read(64)`, `stream(64)`, `read1(64)` and `read(5952)` then `read()`: only raw deflate lost bytes.

Three paths reached the end of the body without that flush:

- `read()` returned early when the raw read was empty, nothing was buffered and `has_unconsumed_tail` was false. That return came before the decode, though `flush_decoder` had just been computed as true.
- The loop that fills a read up to `amt` set `flush_decoder` once, before the loop. When a raw read inside the loop hit the end of the body, the decoder ran with the old value, false. A `deflate, gzip` body read with `read(1)` returned `b""` before the end of the body.
- `stream()` stopped once the underlying response was closed. `http.client.HTTPResponse` closes itself as soon as `Content-Length` bytes are read, so the read that flushes never ran.

## The fix

```diff
             not data
             and len(self._decoded_buffer) == 0
             and not (self._decoder and self._decoder.has_unconsumed_tail)
+            and not (decode_content and self._has_decoded_content)
         ):
             return data
```

The early return is now skipped once a decompress call has run, with or without `amt`. The fill loop now sets `flush_decoder = amt != 0 and not data` after each raw read. `stream()` keeps reading after a decompress call has run, and stops at the first empty read.

One side effect: a truncated zstd body read with `read(amt)` now raises `DecodeError` at the end, as `read()` and `read1()` already did. Before, it could end short with no error. A decoder that never got data, such as an empty zstd body, still returns `b""`.

## The test

Four tests are new. Three of them, six cases in all, fail without the fix and pass with it: `test_deflate2_decodes_whole_body_in_small_reads` runs `read(64)`, `stream(64)` and `read(5952)` then `read()` over an `http.client.HTTPResponse` with `Content-Length`; `test_read_multi_decoding_raw_deflate_gzip_in_small_reads` reads a `deflate, gzip` body one byte at a time; `test_decode_zstd_incomplete_read_with_amt` expects the `DecodeError`. The fourth, `test_decode_zstd_empty_body_read_with_amt`, pins that an empty zstd body still reads as `b""`, and passes either way.

At the merge commit, the selected tests give `7 passed`. With `response.py` from the parent commit and the same tests, they give `6 failed, 1 passed`; the one that passes is the empty-body pin.

## What review changed

Before review, the PR fixed only the fill loop and the early return for `read(amt)`. @illia-v requested changes with two cases it missed:

- With `http.client.HTTPResponse` and a `Content-Length` header instead of `BytesIO`, `stream(64)` still gave 5952 bytes, because the loop stopped when the response closed.
- `read(5952)` followed by `read()` returned no more data, though another `read(64)` got the missing 48 bytes. The first commit bypassed the early return only when `amt` was set.

I fixed both in the next commit, and the test now builds an `http.client.HTTPResponse`. Bypassing the early return for `read()` exposed one more case: a second `read(cache_content=True)` at the end of the body would have replaced the cached body with `b""`. Only non-empty data is cached now, with a deflate row in `test_cache_content_with_explicit_read_call`. @illia-v approved and merged it on 2026-11-11.

## The companion fix

`GzipDecoder.decompress()` also stops once it has produced `max_length` bytes. If that point was exactly the end of a gzip member, the loop broke before it swapped in a new decompressor. The next call handed the remaining input to the finished one, zlib appended it to `unused_data` a second time, and the next member came back over and over. Two 100-byte members, read with `resp.read(100)` by a snippet that stops after ten reads:

```
without the fix: ['x*100', 'y*100', 'y*100', 'y*100', 'y*100', 'y*100', 'y*100', 'y*100', 'y*100', 'y*100']
with the fix:    ['x*100', 'y*100']
```

The fix moves the end-of-member check before the `max_length` break. `test_decode_gzip_multi_member_with_max_length` reads two 1 KiB members with `read(1024)` and with `read1(1024)`. Without the fix both cases fail at the last call, which returns more `y` bytes instead of `b""`; with it they pass. @illia-v approved it ("LGTM, thanks!") and merged it on 2026-11-11.

## The lesson

- A decompressor asked for at most N bytes can stop with output still inside it, and `unconsumed_tail` counts only input. Flush at the end of the body whatever the tail check says.
- A flag that means "this is the last read" must be recomputed after every read in a loop, not once before it.
- Run each body through every read API: `read(amt)`, `stream(amt)`, `read(amt)` then `read()`. Use a real `http.client.HTTPResponse` too. It closes itself once the last `Content-Length` byte is read, so `stream()` stopped before the empty read that flushes. A `BytesIO` body is closed only by that empty read, so tests on `BytesIO` passed with the first version of the fix.

If a reader of yours stops at a byte limit before its decoder is drained, reply on [X](https://x.com/spicysharkco).
