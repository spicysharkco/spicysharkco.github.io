---
layout: post
title: "Config media type used as artifact type: FilterArtifactType in oras-go"
date: 2026-10-20
project: oras-go
pr: https://github.com/oras-project/oras-go/pull/1640
status: merged
tags: [observability-data, go, testing]
---

A copy that used `opts.FilterArtifactType(regexp.MustCompile("sbom"))` skipped a referrer whose `artifactType` was `application/vnd.example.sbom.v1`, and `"empty"` copied it. oras-go is a Go library for managing OCI artifacts. [PR #1640](https://github.com/oras-project/oras-go/pull/1640) fixes it, and @TerryHowe merged it under three hours after I opened it.

## When it happens

The referrer is an image manifest with that `artifactType` and an empty config. It happens when the source's descriptors carry no type, so the filter reads it from the manifest. A `memory.Store` filled with `content.NewDescriptorFromBytes` descriptors does, and so does an OCI layout opened with `oci.NewFromFS`, which keeps only digest, media type and size.

## How the filter finds a type

A predecessor image manifest whose descriptor has no type goes to `fetchArtifactType`, which reads the manifest before the regex runs. For an image manifest it returned the config media type even when `artifactType` was set, so the SBOM was matched against `application/vnd.oci.empty.v1+json`. The distribution spec says, under Listing Referrers:

> If the `artifactType` is empty or missing in the image manifest, the value of `artifactType` MUST be set to the config descriptor `mediaType` value.

So `artifactType` comes first and the config media type is the fallback.

## Why the gap existed

In August 2022 the filter read only artifact manifests. A September 2022 refactor added the image-manifest case and returned `Config.MediaType`; the image-spec build pinned then had no `ArtifactType` on `Manifest`. The field arrived in v1.1.0-rc.3, which oras-go adopted in May 2023, and the image-manifest case stayed as it was. The filter's tests built one image manifest, and it set no `artifactType`.

## The fix

The `extendedcopy.go` diff as merged, trimmed to the added lines (the other 68 lines are the test):

```diff
@@ -434,6 +434,9 @@ func fetchArtifactType(ctx context.Context, src content.ReadOnlyGraphStorage, de
+		if manifest.ArtifactType != "" {
+			return manifest.ArtifactType, nil
+		}
```

## The test

`TestExtendedCopyGraph_FilterArtifactTypeWithRegex_ImageManifest` pushes three referrers into a `memory.Store`: an SBOM with an empty config, a signature with a typed config, and a manifest with no `artifactType` and a typed config. Four rows copy with `sbom`, `signature`, `empty` and `example\.config`, then check all three referrers. In each of three runs, all four failed without the fix and passed with it.

## What the maintainer did

@TerryHowe approved it within minutes and squash-merged it a couple of hours later. The merged change equals mine. Right after the merge they wrote “Thanks!”.

## The lesson

- Search for the old wrong value too. The `empty` row matches the config media type the old code returned and expects nothing copied; a fix that appends that value to `artifactType` fails it and `example\.config` (mutant run).
- Give the fallback its own row. A fix that always returns `artifactType` passes three rows and fails the one whose manifest sets none (mutant run).

If a filter of yours reads one field and falls back to another, reply on [X](https://x.com/spicysharkco).
