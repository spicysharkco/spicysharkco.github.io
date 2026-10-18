---
layout: post
title: "A colour convention that looked like a sim-to-real gap"
date: 2026-10-15
tags: [field-notes, computer-vision, benchmarks]
---

I built a render generator and a 20,000-image benchmark to ask one question. When a perception model is trained on renders and used on a camera, what does the synthetic data buy, and what does it cost? The testbed is chess-position recognition. The labels come from the scene state, so they are exact for the scene, and I can change one factor at a time.

The first result was not about the domain gap. A public checkpoint scored zero exact boards on my renders, and I read that as sim-to-real shift. About two thirds of the wrong squares came from a colour convention, and most boards still fail after the fix. Changing one renderer expression moved per-square accuracy from 54.4 to 85.2. The code and receipts are in the [GitHub repo](https://github.com/spicysharkco/chess-sim2real-benchmark). The development renders are in the [Hugging Face dataset](https://huggingface.co/datasets/spicysharkco/chess-sim2real-benchmark). The dataset holds the 16,000 development images. The 4,000 sealed images are not released, and the 192 probe renders from the earlier lineage are not published, only their receipts.

## The setup

The generator draws a procedural board, a piece family, a perspective camera, and explicit lighting and materials. Labels come from the scene description, never from a detector. A label is a piece-placement FEN, which names all 64 squares. The model under test is the public ChessQueries checkpoint. Its weights are released, so I could score it as is.

## What the gap looked like

My early renders came in 18 splits from one generator lineage, with one board colour convention throughout. On every split the checkpoint produced zero exact boards. Per-square accuracy was 52.7 to 56.7. That is 4.6 to 9.0 points below a constant all-empty prediction, which also gets zero exact boards on these positions. So the model was not defaulting to empty. It was putting pieces on the wrong squares.

The same checkpoint, through the same code path, scored 99.62 exact on ChessReD (2,129 real photographs) and 98.54 on ChessCog, which is also rendered. The failure looked specific to my renders.

I tried the usual input fixes. A crop diagnostic fed the model 450 boards, 150 from each of three splits, four ways: full frame, bounding-box crop, square-padded crop, and a perspective warp to a square. None produced an exact board. Per-square accuracy moved by at most 1.2 points from the full frame. My notes at that point read: "the shift is the piece and board appearance itself."

## Why I believed it

The model was good on photos and on another rendered set, and bad on mine. Cropping and warping did nothing. What was left was appearance, which is the textbook story for a sim-to-real gap. You can tell it without verifying anything.

## What the convention audit recorded

I recorded the board colour convention of one representative image from each of 56 source directories (some are retry or copy lineages). I use `a1_light` for a1 and h8 light, a8 and h1 dark, and `a1_dark` for the reverse. The laws of chess put a dark square at a1. My renderer, following the convention of the data it had been matched to, made a1 light.

In the sampled image, ChessReD, ChessCog, CVChess and ChessRender360 have a8 and h1 light, so a1 is dark. The sampled SLCC frame has no printed coordinates, so I recorded it as unresolved. My own procedural sources were a1 light in every sampled record. I also read the reviewed loaders. None of them mirrors the labels to compensate for board colour.

## The one-expression fix

The old renderer coloured a square from the parity of its row and column: `(r+c)%2===1?lm:dm`. Here `lm` is the light material and `dm` the dark one. The row `r` counts ranks in FEN order, so rank 8 is row 0, and `c` counts files from a. Square a1 is row 7, column 0. Seven is odd, so a1 got `lm`. Swapping the branches makes it dark. The probe applies the swap as a patch to the renderer source. The pattern tolerates whitespace, and the patch refuses to run unless it matches exactly once:

```python
def swap_parity(source):
    pattern = rb"(\(r\+c\)%2===1\s*\?\s*)lm(\s*:\s*)dm(?![A-Za-z_0-9])"
    if len(re.findall(pattern, source)) != 1:
        raise ValueError("expected exactly one original checkerboard assignment")
    return re.sub(pattern, lambda match: match[1] + b"dm" + match[2] + b"lm", source)
```

I rendered 192 positions with and without the patch. The control renders had to be byte-identical to the frozen originals, or the run stopped. Position, camera, piece geometry and projected corners had to match inside each pair, and the two images had to differ. The patch changes the physical checkerboard colours. It does not touch the label order or the image orientation.

## Before and after

The released checkpoint on 192 positions:

| Renders | Model colour indices | Exact boards | Per-square |
|---|---|---|---|
| original (a1 light) | released | 0/192 | 54.36 |
| expression swapped (a1 dark) | released | 18/192 | 85.24 |
| original (a1 light) | flipped in memory | 0/192 | 75.09 |
| expression swapped (a1 dark) | flipped in memory | 4/192 | 82.37 |

The swap removed 67.65% of the wrong squares. The 95% paired bootstrap interval on the +30.88 per-square gain is 28.91 to 32.76. On 288 positions from six other piece families, the swap took exact boards from 0/288 to 50/288 and per-square from 55.31 to 87.49.

The checkpoint carries a learned two-class colour index for its 64 square queries. Flipping it in memory on the original renders recovers 20.7 points but no exact board, and costs 2.9 points on the swapped ones. That fits a colour prior contributing to the failure. It does not isolate the mechanism.

Most boards still fail after the fix. Only 18 of 192 are exact, and in the lowest of four elevation bands none of 48 are. The convention explained about two thirds of the wrong squares. The rest is unexplained. It may be a real domain gap, or camera elevation, framing and piece geometry. This probe cannot say how much.

On the 400-group development partition of the v2 benchmark, the same checkpoint scores 15.48 exact pooled under a1 dark and 0.0 in all twenty cells under a1 light.

## The lesson

Check label and convention alignment before you blame the domain gap. Four checks are cheap.

- Write down which corner is light in your data and in the data the model saw.
- Read the loaders for any reflection of images or labels.
- Treat a score below a constant baseline as a mapping error until shown otherwise. That is my rule of thumb, not a result from the repo.
- Make the convention a factor and render both. In v2 it is the fifth axis, so every scene exists in both conventions.

## What else the benchmark measures

The design has 500 position groups, 400 for development and 100 sealed. Five factors vary one at a time: camera elevation, piece geometry, board palette, framing, and the colour convention. Twenty factor cells at both conventions give 20,000 images, 18,000 of them unique renders. Positions are disjoint from 67 inventoried sources, exactly and under the eight board symmetries. "Sealed" is my no-scoring rule, not an access control. I scored no model on that split.

Three more findings:

- Low-rank adaptation on 500 rendered positions lifted the checkpoint from 15.5 to 64.7 exact on the standard convention (dark-only training), and 8,000 positions reached 79.7 (one seed each). Every one of ten adapted checkpoints then lost 11 to 23 boards on the 2,129 real photos, against a retention gate fixed before training.
- The lowest-elevation a1-dark cell reaches only 14.5 exact after the largest both-convention budget. The reference-elevation cell reaches 86.5.
- A pipeline selected on a private leaderboard scored below its own single member on 16 of 18 controlled splits. Two sets named as holdouts shared 1,446 and 1,128 image identifiers with the training mixes.

If a convention mismatch ever passed as a domain gap in your pipeline, I would like to hear how you found it. Reply on [X](https://x.com/spicysharkco).
