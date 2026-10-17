---
layout: post
title: "Two URL lists: the adapter webhook and the runtime sidecar"
date: 2026-10-17
---

I found that AIBrix could admit an adapter URL and then reject it when the runtime sidecar tried to load it. I fixed two forms: the `hf://` alias in [PR #3130](https://github.com/vllm-project/aibrix/pull/3130), merged 2026-10-16 about 2 hours 27 minutes after I opened it, and an absolute path starting with `/` in [PR #3131](https://github.com/vllm-project/aibrix/pull/3131), merged 2026-10-17 about 6 hours 5 minutes after opening. The webhook and the sidecar kept separate lists of acceptable inputs.

## When it happens

Both failures depended on the controller using the runtime sidecar. With that path enabled and an `aibrix-runtime` container present, the controller sent the adapter URL to the sidecar. The direct path handled the same inputs differently: it resolved the alias to a repository identifier and passed an absolute path through to the engine.

The alias failure was narrower than a broken downloader. The longer `huggingface://` spelling already worked. The docs and samples used that spelling, while the webhook's supported-input error advertised `hf://` too. For absolute paths, the loading guide explicitly told readers to mount shared storage into the pod and use a value starting with `/`. I was following an accepted input to its consumer, rather than adding a new URL form.

## How the value travels

In `pkg/webhook/modeladapter_webhook.go`, the webhook parses `artifactURL` and calls `ValidateArtifactURL`. Its list lives in `pkg/utils/modeladapter.go`: `AllowedSchemas` includes both `hf://` and `/`.

The controller's `pkg/controller/modeladapter/lora_client.go` then chooses the load payload. For the sidecar, `loadAdapterCall` puts the original value into `artifact_url`. It does not turn the alias into the longer spelling or change a mounted path into a remote URL.

Next comes `load_adapter_with_delegation` in `python/aibrix/aibrix/runtime/artifact_service.py`. Before my changes, it sent every value through `download_artifact`. That reached `get_downloader` in `python/aibrix/aibrix/runtime/downloaders.py`, which selected a downloader by the parsed scheme. There was an entry for `huggingface`, none for `hf`, and none for the empty scheme of an absolute path. The webhook's acceptance did not carry across that boundary.

## Why the lists drifted

[#556](https://github.com/vllm-project/aibrix/pull/556) added absolute-path support to the controller and its validation. Later, [#1680](https://github.com/vllm-project/aibrix/pull/1680) introduced downloading through the runtime. Its downloader map had the longer spelling but no alias, and its delegation method sent all inputs to the downloader.

## The fixes

For the alias, I added the missing dispatch entry:

```diff
     "huggingface": HuggingFaceArtifactDownloader(),
+    "hf": HuggingFaceArtifactDownloader(),
```

The downloader already built the repository identifier from the URL's netloc and path. It did not need a different download implementation for the alias.

For a mounted path, there is nothing to download. The merged branch is:

```python
if artifact_url.startswith("/"):
    self._validate_lora_name(lora_name)
    local_path = artifact_url
else:
    local_path = await self.download_artifact(
        artifact_url, lora_name, credentials
    )
```

The engine receives that value as `lora_path`. It resolves the path in its own container. The runtime does not copy the mounted directory or check that it exists there.

## The tests

My PR descriptions record the local before-and-after runs. For the alias, the focused file had `1 failed, 2 passed` before the fix and `3 passed` after it. The runtime suite reported `178 passed`. That test uses a stand-in for the download call; it checks dispatch and the returned directory, without a real Hub download.

For the local-path change, the final focused file had `2 failed, 2 passed` before and `4 passed` after. The failures were the path-load cases. The other cases checked invalid names. The full runtime directory went from `2 failed, 179 passed` to `181 passed` in the runs recorded for that PR.

The path tests use a recording engine. They check the forwarded path and that the runtime writes nothing into its download directory. They do not prove a real engine loaded files from a mounted volume.

## What happened in review

On the alias PR, the review reproduced the failure and suggested sharing a downloader instance and listing the alias in docstrings. Those were optional suggestions; the merged change contains the dispatch entry and its test.

The local-path review found a more useful mistake in my first version. A review bot pointed out that bypassing `download_artifact` also bypassed name validation. The review confirmed it: a name containing a slash could load through the new branch, then produce a confusing cleanup error after the engine had already unloaded it.

I added the explicit validation call in [c8ce9b77](https://github.com/vllm-project/aibrix/commit/c8ce9b77f1921b54857f963ebdf0ce78fb48f9c4), plus a test requiring local and remote inputs to reject the same invalid name before reaching the engine. The follow-up review removed that call and saw the local case fail. @varungup90 merged both PRs.

## The lesson

- Trace each accepted spelling to its final consumer. A validator's list is only useful if later dispatch agrees.
- Before bypassing a helper, list its other jobs. I needed to preserve name validation when skipping the download.
- Keep the evidence boundary clear. Dispatch tests and a recording engine answer useful questions without proving a mounted-volume load.

If you have found an input accepted at the front door and rejected later, reply on [X](https://x.com/spicysharkco).
