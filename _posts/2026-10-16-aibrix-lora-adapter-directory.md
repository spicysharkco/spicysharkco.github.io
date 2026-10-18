---
layout: post
title: "Two directories for one adapter: a Hugging Face download fix in AIBrix"
date: 2026-10-16
project: AIBrix
pr: https://github.com/vllm-project/aibrix/pull/3126
status: merged
tags: [model-serving, lora, hugging-face]
---

AIBrix's runtime handed the inference engine a different directory for the same LoRA adapter, depending on whether the adapter had been downloaded before. I fixed it by changing one argument ([PR #3126](https://github.com/vllm-project/aibrix/pull/3126), merged 2026-10-16, under three hours after I opened it). I found it while reading the runtime's artifact service. I found no open issue for it.

## Two exits, two values

`download_artifact` in `python/aibrix/aibrix/runtime/artifact_service.py` has two ways to return a path. On the first call it runs the downloader and returns the path the downloader reports. Once a `.aibrix_download_complete` marker exists, it returns `local_path`, the directory it gave the downloader. For S3, GCS, TOS and HTTP those are the same directory. For Hugging Face they were not.

The Hugging Face downloader called `snapshot_download(repo_id=..., cache_dir=local_path)`. With `cache_dir`, `huggingface_hub` writes the files under `local_path/models--<org>--<name>/snapshots/<revision>/` and returns that snapshot directory. So the first load gave the engine the snapshot directory, and every later load gave it `local_path`, which has no `adapter_config.json` at its root. Later loads happen: a retry after an engine error, a repeated load request, a restart that keeps the adapter volume, an unload with `cleanup_local=false`.

## The fix

`cache_dir` means "keep a cache here". The option for "put the files in this directory" is `local_dir`, and the repo's model downloader in `aibrix/downloader/huggingface.py` already used it. The whole change to `downloaders.py`:

```diff
-            # Download model/adapter
+            # Download model/adapter. Use local_dir so the files land directly
+            # in local_path, like the other downloaders. With cache_dir they
+            # would sit under models--<org>--<name>/snapshots/<revision>/, a
+            # path the artifact service does not return once the completion
+            # marker exists.
             download_path = snapshot_download(
                 repo_id=repo_id,
-                cache_dir=local_path,
+                local_dir=local_path,
                 token=token,
             )
```

## Evidence

- I checked the layouts on `huggingface_hub` 0.24.x with a small public repo. `cache_dir=/x/a` returned `/x/a/models--<org>--<name>/snapshots/<sha>`, and `local_dir=/x/b` returned `/x/b` with `config.json` directly inside.
- With the change, I called `download_artifact` twice against the real Hub. Both calls returned the same directory containing the file.
- A new test file uses a stand-in for `snapshot_download` that follows both layouts. One test checks that the files land in the adapter's directory. The other calls `download_artifact` twice and compares the results. Before the change both failed: one got the snapshot path where it expected `.../adapter`, the other got a different path on the reload. After it, 2 passed. The review also ran all of `tests/runtime` with no failures.

None of my checks involved an inference engine. That an engine would reject a directory without `adapter_config.json` at its root is an inference from the layout. The review listed a real engine loading an adapter among the things it had not run either.

## What review asked for

**A flag to add.** A review bot [suggested](https://github.com/vllm-project/aibrix/pull/3126#discussion_r4297357847) adding `local_dir_use_symlinks=False`, on the worry that `local_dir` leaves symlinks into a cache that breaks in containers. I [replied](https://github.com/vllm-project/aibrix/pull/3126#discussion_r4297426435) that in the version `poetry.lock` pins, 0.24.7, the parameter is deprecated and ignored: `hf_hub_download` only warns, and the files are written into `local_path` itself. Adding the flag would add the warning and nothing else. The review, posted from the `varungup90` account, called that reply accurate and said it had read the 0.24.7 wheel to check. The repo's model downloader does pass that flag, so the suggestion matched what the code next door does. The review agreed that leaving it out was right.

**Two strings for one directory.** The review's first optional follow-up: `snapshot_download(local_dir=...)` returns the real path of `local_dir`, while the marker path returns `local_path` as given. They are the same directory, but if the adapters directory is reached through a symlink they are different strings. The review suggested returning `local_path` from both branches. I [agreed](https://github.com/vllm-project/aibrix/pull/3126#issuecomment-6223413663) and offered to add it to the PR or leave it for later, as the review had proposed. The merged PR has its one commit and no `realpath` change.

**What the fix does not repair.** The second follow-up: a directory downloaded under the old layout, with its marker, keeps returning `local_path`. If a deployment keeps adapters on a persistent volume, the review wrote, this "will bite after the upgrade", and a release note would help. My description already had a Compatibility section saying this. In my reply I added that nothing regresses, because the marker path returned `local_path` before the change too, so such an adapter was already broken on its first reload. It needs `cleanup_local`, or removing the directory, once.

The review also reproduced the failure with the base file and my tests. It checked that the resume files a download leaves under `.cache/huggingface/` cannot make a failed download look complete, because the content check runs only after the downloader returns successfully.

When the review looked, the Python test jobs and image builds were still queued. By the time I commented, they had passed. The review recommended approval but was posted as a comment, and @varungup90 merged the PR 30 minutes after my CI comment.

## The lesson

- If a function has a fast path and a slow path, test it by calling it twice and comparing the results. The reload test in this PR is two calls and a comparison.
- An option named `cache_dir` is not an output directory. Check what the call returns and where the files land, and read the source of the version your lockfile pins, not the latest documentation.
- When a bot suggests a flag, check whether your pinned version still honors it. Answer with the version and what its code does.
- Say what the change leaves behind. State written by the old code is where a fix goes quiet, and a reviewer is likely to ask about it.

If a cached path in your own code came back different the second time, reply on [X](https://x.com/spicysharkco).
