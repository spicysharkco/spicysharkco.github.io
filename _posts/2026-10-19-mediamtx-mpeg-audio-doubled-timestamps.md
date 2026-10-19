---
layout: post
title: "Doubled timestamps cut a recording every 5 seconds: MPEG audio in mediamtx"
date: 2026-10-19
project: mediamtx
pr: https://github.com/bluenviron/mediamtx/pull/6361
status: merged
tags: [media-vision, recording, audio]
---

A 20-second MP3 stream became three recording files in mediamtx, and audio between them was lost. The recorder doubled the timestamps of MPEG audio and cut the recording every 5 seconds. [PR #6361](https://github.com/bluenviron/mediamtx/pull/6361) fixed it and was merged about two days after I opened it.

## When it happens

A path has `record: yes` and `recordFormat: fmp4`. A client publishes MP3 or MP2 (MPEG-1/2 Audio) at its real pace. After 5 seconds the log says `detected drift between recording duration and absolute time, resetting`, and a new recording starts 2 seconds later. The cycle repeats: three files, each with about 5 seconds of audio that it says lasts about 10.

## How the recorder counts time

The recorder takes a sample's duration from the next sample's DTS (decode time). It also compares absolute time with the DTS time since the start. Past `ntpDriftTolerance`, 5 seconds, `formatFMP4Track.write` returns the drift error and the recording resets.

## Why the gap existed

In January 2024 this case changed `dts: pts` to PTS plus PTS and renamed its running sum `pts` to `dt`, which nothing read. DTS time ran at twice the clock, so the gap reached 5 seconds at second 5. The drift check came in December 2025.

## The fix

Hunks of the MPEG-1/2 Audio case in `format_fmp4.go`, trimmed to changed lines, names after `@@` removed:

```diff
@@ -732 +732 @@
-						var dt time.Duration
+						pts := u.PTS
@@ -752,2 +752,2 @@
-								dts: u.PTS + u.PTS,
-								ntp: u.NTP,
+								dts: pts,
+								ntp: u.NTP.Add(timestampToDuration(pts-u.PTS, clockRate)),
@@ -759,2 +759 @@
-							dt += time.Duration(h.SampleCount()) *
-								time.Second / time.Duration(h.SampleRate)
+							pts += int64(h.SampleCount()) * int64(clockRate) / int64(h.SampleRate)
```

`pts` starts at the unit's PTS, grows by each frame's length in clock ticks, and is the DTS. The NTP moves by the same offset.

## The test

`TestRecorderFMP4MPEG1AudioDuration` writes three units of two frames. A frame of 1152 samples at 44.1 kHz lasts 2351 ticks; the units start 4702 apart. It reads the parts back and checks that all five samples last 2351 ticks. The newest frame is held, so six frames give five samples.

Before the fix it failed with `expected: 0x92f` and `actual  : 0x0`. After it, 10 of 10 runs passed, and `-race` passes. A 20-second run at real pace with the fix gave one file, no drift line.

## What the maintainer did

@aler9 merged it as one commit with the comment “merged, thanks!”. My changes match the merged ones.

## The lesson

- Run the stream at its real pace once. The wrong field was a duration, but the harm was a cut: the drift check compares recorded time with the clock.
- When a fix copies a sibling, copy its whole idiom: the Opus case moves `dts` and `ntp` together. The test checks durations only, so it passes without the `ntp` line.

If a recorder of yours checks time against a clock, reply on [X](https://x.com/spicysharkco).
