---
layout: page
title: Research
permalink: /research/
---

## Chess sim-to-real benchmark

**Code.** A controlled render generator and a 20,000-image benchmark for chess-position recognition, built to measure what synthetic training data buys and what it costs when a perception model trained on renders is used on a camera. The repository holds the generator, the benchmark v2 manifests (500 position groups, 400 development and 100 sealed), the scoring tools and the licence files. [spicysharkco/chess-sim2real-benchmark on GitHub](https://github.com/spicysharkco/chess-sim2real-benchmark)

**Dataset.** The 16,000 development images of the benchmark: 1920 x 1080 PNG renders of 400 position groups, each with its position as a piece-placement FEN and its projected board corners. The 4,000 images of the sealed final-test split are not included, and the dataset holds renders only. [spicysharkco/chess-sim2real-benchmark on Hugging Face](https://huggingface.co/datasets/spicysharkco/chess-sim2real-benchmark)
