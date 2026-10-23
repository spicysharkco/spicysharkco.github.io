---
layout: page
title: Research
permalink: /research/
---

## Chess sim-to-real benchmark

**Code.** A controlled render generator and a 20,000-image benchmark for chess-position recognition, built to measure what synthetic training data buys and what it costs when a perception model trained on renders is used on a camera. The repository holds the generator, the benchmark v2 manifests (500 position groups, 400 development and 100 sealed), the scoring tools and the licence files. [spicysharkco/chess-sim2real-benchmark on GitHub](https://github.com/spicysharkco/chess-sim2real-benchmark)

**Dataset.** All 20,000 images of the benchmark: 1920 x 1080 PNG renders of 500 position groups, each with its position as a piece-placement FEN and its projected board corners. The 16,000 development images of 400 position groups are the default configuration. The 4,000 images of the sealed final-test split, the other 100 groups, were added on 2026-10-23 as the configuration `final_test`; sealed means we have not scored any model on them. The dataset holds renders only. [spicysharkco/chess-sim2real-benchmark on Hugging Face](https://huggingface.co/datasets/spicysharkco/chess-sim2real-benchmark)

## Qwen3.8-27B-OBLITERATED, dynamic GGUF

**Model.** GGUF quants of [OBLITERATUS/Qwen3.8-27B-OBLITERATED](https://huggingface.co/OBLITERATUS/Qwen3.8-27B-OBLITERATED) (V3), an abliterated Qwen3.8-27B, where each tensor group gets its own quant type, chosen by measured sensitivity, inside a fixed file-size budget. Three files: DYN-Q4 (16.81 GB), DYN-Q4-S (16.46 GB) and DYN-Q3 (13.49 GB). [spicysharkco/Qwen3.8-27B-OBLITERATED-DYN-GGUF on Hugging Face](https://huggingface.co/spicysharkco/Qwen3.8-27B-OBLITERATED-DYN-GGUF)

**Results.** Each file is scored by KL divergence from the BF16 model on the same 30,690 held-out tokens (half wikitext-2, half chat conversations), with the llama.cpp CPU backend. The upstream Q4_K_M and Q3_K_M match DYN-Q4 and DYN-Q3 in size within 0.01 GB. Lower KL divergence is better.

| file | size GB | mean KLD | p99 KLD | same top token % |
|---|---|---|---|---|
| upstream Q4_K_M | 16.81 | 0.0348 | 0.335 | 93.84 |
| DYN-Q4 | 16.81 | 0.0195 | 0.227 | 95.33 |
| DYN-Q4-S | 16.46 | 0.0238 | 0.282 | 94.89 |
| upstream Q3_K_M | 13.50 | 0.1118 | 1.283 | 87.65 |
| DYN-Q3 | 13.49 | 0.0621 | 0.697 | 91.37 |

**Where the gain comes from.** A file with llama.cpp's stock Q4_K_M recipe and the importance matrix DYN-Q4 uses scores 0.0247 mean KLD, so about two thirds of DYN-Q4's gain comes from the importance matrix and the rest (a further 21% lower, from 0.0247 to 0.0195) from the per-tensor mix. The evaluation is text only; no downstream task benchmark was run.
