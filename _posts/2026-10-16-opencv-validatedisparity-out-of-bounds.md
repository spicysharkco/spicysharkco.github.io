---
layout: post
title: "An index that trusted the disparity map: the validateDisparity fix in OpenCV"
date: 2026-10-16
---

My fix for an out-of-bounds access in OpenCV's `cv::validateDisparity` was merged into the 4.x branch today as [PR #30316](https://github.com/opencv/opencv/pull/30316). It answers [issue #25367](https://github.com/opencv/opencv/issues/25367), filed in April 2024 as "crash and Unknown C++ exception when I invoke cv2.validateDisparity". Two earlier PRs for it were closed. The issue's snippet does not crash for me. The bug is real anyway, and a different input shows it.

## What the function does

It is the left-right consistency check for a disparity map. The header comment in `modules/calib3d/include/opencv2/calib3d.hpp` says:

> validates disparity using the left-right check. The matrix "cost" should be computed by the stereo correspondence algorithm

`StereoBM` calls it after matching when `disp12MaxDiff >= 0`, with its disparity map and a `CV_16S` map of the best SAD score per pixel (`modules/calib3d/src/stereobm.cpp`, lines 1122-1123 and 1237). `StereoSGBM` does its own check inline, in `stereosgbm.cpp` from line 2147.

Disparities are `CV_16S` fixed point, 16 units per pixel. For each row, `validateDisparity` ([`stereosgbm.cpp`, line 2514](https://github.com/opencv/opencv/blob/607d01b1ddf8278008d6410d1522a248f26c6e90/modules/calib3d/src/stereosgbm.cpp#L2514)) makes two passes:

1. For each pixel x with disparity d, find where it lands in the other image, `x2 = x - ((d + 8) >> 4)`. Per landing column, keep the disparity of the lowest-cost pixel. Two `cols`-long int arrays, `disp2buf` and `disp2cost`, hold this in one allocation.
2. For each pixel, round d down and up to two columns and look both up. If both entries exist and both differ from d by more than `disp12MaxDiff`, the pixel becomes invalid, which is `(minDisparity - 1) * 16`.

The first pass runs x from `max(maxD, 0)` to `cols + min(minD, 0)`. A disparity inside `[minDisparity*16, (minDisparity+numberOfDisparities)*16)` always gives `x2` in `[0, cols)`. I checked 5,433,296 cases by brute force (columns 20, 50 and 160, `minDisparity` -6 to 6, 1 to 16 disparities). The bounds hold only if the data keeps that promise.

## The input that broke it

A disparity outside that range, for example from a map computed with other parameters, or a negative one. Then `x2` can go negative or reach `cols`, and the code indexed with it unchecked, in both the `CV_16S` and the `CV_32S` cost branches.

Two cases from my test, on a row of 160 columns. With `minDisparity = 2` and `numberOfDisparities = 16`, pixel 159 with d = 2672 gives x2 = 159 - 167 = -8. With `minDisparity = 0`, pixel 159 with d = -32 gives x2 = 161.

The two directions fail differently. `disp2cost` starts at `disp2buf + cols`, so `disp2cost[-8]` is `disp2buf[152]`, a live entry for other pixels: the write silently changes another pixel's answer. Its partner write, `disp2buf[-8]`, lands 32 bytes before the buffer. `disp2cost[161]` is 4 bytes past the end of the 320-int buffer.

The issue's snippet uses random 100x100 maps with values 0 to 255, `minDisparity = 3` and `numberOfDisparities = 7`. Enumerating that range, x2 runs from -6 to 99, so only the negative side is reachable. With 100 columns the buffer is 200 ints, which fits inline in `AutoBuffer<int>`, 264 by default (`modules/core/include/opencv2/core/utility.hpp`, line 100). The `disp2cost[x2]` writes land on slots 94 to 99 of the same array. The paired `disp2buf[x2]` writes land before the array: x2 of -4 and -3 overwrite the `ptr` field of the `AutoBuffer` object, and -2 and -1 overwrite `sz`. `~AutoBuffer` calls `delete[] ptr` when `ptr != buf`, so a damaged `ptr` can end in a bad free. That would fit the reporter's exit code, 0xC0000374 (heap corruption), but I could not test their MSVC build. I ran the snippet 100 times each on the 4.9.0 and 5.0.0 Python wheels, macOS arm64: all 200 runs ended with exit status 0. In the 4.9.0 wheel the compiled code keeps `ptr` in a register and never reloads it, so the damage goes unseen.

This line has failed before. [PR #4190](https://github.com/opencv/opencv/pull/4190), merged in 2015, skipped `d == INVALID_DISP_SCALED` because "x2 could be out of bounds if, for instance, x==cols-1 and d==INVALID_DISP_SCALED". Mine covers the other values.

## The diff

Six added lines, the same three in each branch:

```diff
                 int x2 = x - ((d + DISP_SCALE/2) >> DISP_SHIFT);
 
+                if( (unsigned)x2 >= (unsigned)cols )
+                    continue;
+
                 if( disp2cost[x2] > c )
```

Casting to unsigned turns a negative index into a huge one, so one comparison covers both ends. A skipped pixel does not vote in the reverse map, and is not itself changed: in the test it keeps its input value.

## How the test proves it

`Calib3d_ValidateDisparity.out_of_range_disparity` runs one 160-column row for `CV_16S` and `CV_32S`, in both directions: four cases. It compares the whole row with the expected one. The negative case also shows up without a sanitizer:

```
pixels.push_back({154, 32, 0, 32});
pixels.push_back({155, 48, 10, invalid});
pixels.push_back({159, 2672, 0, 2672});
```

Pixels 154 and 155 both map to slot 152, and 154 is cheaper, so 155 fails the cross-check. Without the fix, pixel 159 overwrites slot 152 and 155 is not invalidated. Per the PR, without the fix all four cases fail under ASan, and the two negative ones also fail without a sanitizer. With the fix all four pass, and the existing `StereoBM` and `StereoSGBM` tests pass under ASan.

The work buffer is on the heap only above 132 columns (2 x 133 = 266 > 264). A narrower row sits inside the `AutoBuffer` object, where ASan has no redzone.

## The earlier attempts and the review

[#28507](https://github.com/opencv/opencv/pull/28507) was closed by its author the day it opened: "Closing this PR in favor of a clean re-submission." [#28508](https://github.com/opencv/opencv/pull/28508) was closed in March 2026, and its thread gives no reason. Before that, @asmorkalov saw no crash on Linux, with or without ASan, and asked for a test that reaches the guarded index.

On #30316, @asmorkalov approved with no text and merged. Right after the merge he asked: "Sounds like the same issue exists in 5.x. Could you submit complementary PR there?" It does: the 5.x copy in `modules/stereo/src/stereosgbm.cpp` still has the unguarded lines as I write. That port is the open item.

## Run it

The wheels I tried have no fix. With the fix, the PR's test expects pixel 155 to be 16 in both columns; I did not rebuild OpenCV to print that.

```python
import subprocess
import sys

import cv2
import numpy as np

COLS, MIN_D = 160, 2
INVALID = (MIN_D - 1) * 16  # "no disparity" marker


def pixel_155(bad, dtype):
    disp = np.full((1, COLS), INVALID, np.int16)
    cost = np.zeros((1, COLS), dtype)
    disp[0, 154] = 32                    # 154 and 155 both map to column 152
    disp[0, 155], cost[0, 155] = 48, 10  # 154 is cheaper, so 155 must lose
    if bad:
        disp[0, 159] = 2672              # valid range is [32, 288)
    return int(cv2.validateDisparity(disp, cost, MIN_D, 16, 0)[0, 155])


for dt in (np.int16, np.int32):
    print(np.dtype(dt).name, pixel_155(False, dt), pixel_155(True, dt))

# x2 past the last column, in a child process
child = (
    "import cv2, numpy as np\n"
    "disp = np.full((1, 160), -16, np.int16); disp[0, 159] = -32\n"
    "cv2.validateDisparity(disp, np.zeros((1, 160), np.int16), 0, 16, 0)\n"
)
print("exit status:", subprocess.run([sys.executable, "-c", child]).returncode)
```

On 4.9.0 and on 5.0.0 it printed:

```
int16 16 48
int32 16 48
exit status: 0
```

The two numbers are pixel 155 without and with the far-off pixel; it should be 16. The past-the-end case exits with status 0 and no message.

## The lesson

If a per-pixel loop turns a value from the map into an index, check the index where you use it. The loop bounds came from parameters, and parameters do not constrain the data. A map comes from another stage with other settings. Its invalid marker may fall outside this call's range, and only this call's own marker is skipped.

Make the test fail first: run it against the unfixed code. Mine needed 160 columns and a pixel whose result another pixel can change.

For how others solve it, look at the neighbours. The final loop of this same function range-checks `x0` and `x1`, and the `StereoSGBM` inline check does the same with `0 <= _x && _x < width`. The 2015 fix and this one both check at the use site.

If you have hit a disparity or depth loop that read past a row, I would like to hear how you found it. Reply on [X](https://x.com/spicysharkco).
