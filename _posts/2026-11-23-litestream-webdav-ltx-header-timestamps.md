---
layout: post
title: "A server modification time used as the LTX timestamp: LTXFiles in the WebDAV client of Litestream"
date: 2026-11-23
project: Litestream
pr: https://github.com/benbjohnson/litestream/pull/1576
status: merged
tags: [data, go, webdav]
---

Litestream's WebDAV replica client listed every LTX file with the server's modification time as its creation time. A timestamp-based restore compares that time to the target, so it could pick the wrong file or find none. I sent the fix ([PR #1576](https://github.com/benbjohnson/litestream/pull/1576), merged 2026-10-27). The PR links no issue; it points to [PR #1575](https://github.com/benbjohnson/litestream/pull/1575), a CI change to the integration test jobs.

## When it happens

`LTXFiles` takes a `useMetadata` flag. `CalcRestorePlan` passes `!timestamp.IsZero()` for it, so `litestream restore -timestamp` asks for accurate times and then skips any file whose `CreatedAt` is not before the target. The WebDAV client took the argument as `_ bool` and always set `CreatedAt` from `fi.ModTime()`. That time comes from the server, not from the file. The real time is the timestamp in the LTX header, inside the file, and the listing never opened it.

The PR description reports that with `-replica-clients=webdav` against the job's `bytemark/webdav` server, three timestamp tests failed on `main`. `TestReplicaClient_TimestampPreservation` writes a header timestamp one hour in the past, and its result was off by `59m59.647s`.

## Why the gap existed

Two things. S3 and OSS read per-object metadata when the flag is set, with `HeadObject` calls, 50 at a time by default. WebDAV had no such step.

And the shared integration tests never ran against WebDAV. The job ran `go test -v ./replica_client_test.go -integration webdav`. Here `webdav` is a bare argument, and the `-replica-clients` flag defaults to `file`, so the job ran only the file client. The fix changes the line to `-integration -replica-clients=webdav`.

## The fix

`LTXFiles` now takes `useMetadata` by name and, when it is true, calls a new method before sorting:

```go
	if useMetadata {
		if err := c.fetchTimestamps(ctx, infos); err != nil {
			return nil, err
		}
	}
```

```go
func (c *ReplicaClient) fetchTimestamps(ctx context.Context, infos []*ltx.FileInfo) error {
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(metadataConcurrency)

	for _, info := range infos {
		g.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}

			hdr, err := litestream.FetchLTXHeader(groupCtx, c, info)
			if err != nil {
				if ctxErr := groupCtx.Err(); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("read ltx header timestamp for level %d file %s: %w", info.Level, ltx.FormatFilename(info.MinTXID, info.MaxTXID), err)
			}
			info.CreatedAt = time.UnixMilli(hdr.Timestamp).UTC()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}
```

`FetchLTXHeader` opens the file with `OpenLTXFile(ctx, info.Level, info.MinTXID, info.MaxTXID, 0, ltx.HeaderSize)`, one ranged GET per file. If a server ignores `Range`, the reader is still limited to the header size. `metadataConcurrency` is 10, a constant, not a config field like S3's `MetadataConcurrency`.

## The test

Four tests are new, six cases in all. `TestReplicaClient_LTXFiles_UseMetadata` runs with `RangeSupported` and `RangeIgnored`. The fake server stamps its listing with the current time, the files carry header timestamps about an hour earlier, and `CreatedAt` must equal the header time. `TestReplicaClient_MetadataFailure` answers the GET for one file with status 200 and header-sized junk, or with 503. `TestReplicaClient_LTXFiles_UseMetadata_ContextCanceled` passes a canceled context. `TestReplicaClient_LTXFiles_UseMetadata_ContextCanceledDuringRead` has the server cancel the context on the first GET, then answer normally.

With `replica_client.go` from the parent commit and the merged tests, all four fail, for example `expected metadata error` and `err=<nil>, want context canceled`. At the merge commit, all pass.

I also broke the merged code on purpose to check the tests. Replacing the last `return ctx.Err()` with `return nil` fails only the cancel-during-read test. Replacing the `fmt.Errorf` return in the goroutine with `return nil` fails both `MetadataFailure` cases.

## What the maintainer changed

The PR had no review comments and no reviews. @corylanou pushed two commits to my branch and merged it. My version kept the modification time for a file whose header could not be read and logged a warning. One commit, on 2026-10-20, replaced that with the `fmt.Errorf` above, so an unreadable header now fails the listing. Another, on 2026-10-26, split `groupCtx` from `ctx` and added the final `return ctx.Err()`. gowebdav's `ReadStreamRange` takes no context, and in the test the header read still finishes after the cancel. Without that last line the listing returns success.

The PR description says an unreadable header keeps its modification time and logs a warning. The merged code does not. This post follows the merged code.

A fallback would give that one file the server's time again, the value this PR removes, and the restore would choose by it with only a warning in the log. The tests assert that `FindLTXFiles` with `useMetadata` true returns an error, and so does `CalcRestorePlan` with a timestamp, which returns no plan. A 503 gives `read ltx header timestamp for level 0 file 0000000000000002-0000000000000002.ltx: open ltx file: ...`, and junk bytes give `...: peek header: invalid LTX file`.

## What it does not change

A listing with `useMetadata` false opens no files; `TestReplicaClient_MetadataFailure` checks that it still returns both files. Timestamp restore costs one request per listed file, 10 at a time. The PR description reports that listing 999 files took 0.2 to 0.4 s with metadata and about 0.05 s without.

WebDAV now differs from S3 and OSS. Their `HeadObject` errors are non-fatal and keep `LastModified`; here one unreadable file fails the listing for its level.

## The lesson

- A listing's modification time comes from the server, not from the data. If a restore chooses by time, read the time stored in the data.
- An interface flag that one implementation takes as `_` is ignored without any error. Look for it where several backends share a contract.
- Check that a CI job exercises the backend it is named for. A bare argument where a flag was needed ran the default client.
- When the fallback for a failed read is the wrong value the change exists to remove, return the error.

If something of yours picks files by a time the server assigns, reply on [X](https://x.com/spicysharkco).
