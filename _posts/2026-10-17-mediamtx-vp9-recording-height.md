---
layout: post
title: "I made mediamtx record the real height of VP9 streams"
date: 2026-10-17
---

mediamtx wrote the wrong height into every fMP4 recording of a non-square VP9 stream. I fixed it in [PR #6359](https://github.com/bluenviron/mediamtx/pull/6359), merged within fifteen minutes of opening it. The height was read from the width.

## When it happens

A path has `record: yes`, and `recordFormat` is `fmp4`, the default. A client publishes a VP9 track. For a 1920x804 stream, reading the init segment back gave `Width: 1920` and `Height: 1920`. The height always equalled the width.

I dumped one recorded segment before and after the fix. Before, the `tkhd` box and the `vp09` sample entry both said 1920x1920. After, both said 1920x804.

The playback server builds its init from the tracks of the first recorded segment (`internal/playback/on_get.go`), so it carries the same height.

## How the recorder reads the size

In `internal/recorder/format_fmp4.go`, the VP9 case parses the header of each frame and, on a key frame, copies the size to the codec parameters. The width block calls `h.Width()`. The height block under it called `h.Width()` too. The height variable is named `h`, like the header, so the line read `h := h.Width()`.

## Why the gap existed

The line came with the first version of the recorder, in [PR #2255](https://github.com/bluenviron/mediamtx/pull/2255), merged in September 2023. No test in the recorder package mentioned VP9, so nothing read a VP9 size back.

## The fix

Call `h.Height()`. The hunk from `format_fmp4.go`:

```diff
@@ -252,7 +252,7 @@ func (f *formatFMP4) initialize() bool {
 								codec.Width = w
 								paramsChanged = true
 							}
-							if h := h.Width(); codec.Height != h {
+							if h := h.Height(); codec.Height != h {
 								codec.Height = h
 								paramsChanged = true
 							}
```

## The test

`TestRecorderFMP4VP9Size` in `recorder_test.go` writes key frames of a 1920x804 stream to a recorder and reads the init segment of the recorded file back with `fmp4.Init.Unmarshal`. It writes two key frames, because with one the recorder creates no file. A square size would have passed on the old code, since the width and the height are the same number there.

Before the fix the test failed with `Height: (int) 1920` where it expected 804, in 3 of 3 runs. After it, 10 of 10 runs passed, and the whole recorder package passes with `-race`. Putting back `h.Width()` fails the test again.

## What the maintainer did

@aler9 merged it with the comment “merged, thanks”. It was my first mediamtx PR.

## The lesson

- When two lines are copies of each other, test them with values that differ. A square size cannot tell a width from a height.
- Read the written file back. The wrong number sat in the init segment, where the playback server reads it too.
- A codec case with no test can keep a typo for years. This one stood for three.

If a copied pair of width and height lines sits in your own code, reply on [X](https://x.com/spicysharkco).
