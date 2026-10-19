---
layout: post
title: "Infinity fetched any URL a client sent it: SSRF fixed in 0.0.78"
date: 2026-10-19
---
The Infinity embedding server (`infinity-emb` on PyPI) downloads images and audio from URLs
that clients send it. Through version 0.0.77 it did that without checking where the URL
pointed, and a default install has no authentication. Version 0.0.78 fixes the fetch.
I am requesting a CVE ID and will add it here when assigned.

## What is affected
- **Product:** `infinity-emb`, the Python server (v1) in michaelfeil/infinity. The separate
  Rust v3 line is not covered here.
- **Affected:** 0.0.41 through 0.0.77. I compared the fetch code in each tag: the image
  fetch is present from 0.0.41 and the audio route from 0.0.58. I did not run each version.
- **Fixed:** 0.0.78.
- **Class:** server-side request forgery (CWE-918), reachable without authentication on a
  default install.
- **Precondition:** the server has to be serving a model that accepts image or audio input.

## Where it was
In `libs/infinity_emb/infinity_emb/transformer/vision/utils.py`, `resolve_from_img_url`
passed the client's URL to an HTTP session and read the body. `resolve_audio` in
`transformer/audio/utils.py` did the same for audio. `POST /embeddings` with image or audio
input given as a URL reaches them, as do the deprecated `/embeddings_image` and
`/embeddings_audio` routes. Nothing checked the destination or the redirects, the session
honoured proxy settings from the environment, and the download had no size limit.

The auth gate is only attached when `INFINITY_API_KEY` is set, and it is empty by default.
By default the server binds to `0.0.0.0:7997`.

## Why it matters
The server makes the request from wherever it runs. Anyone who can reach the port can ask it
to contact addresses they cannot reach themselves: services on loopback, hosts on the
private network, the link-local metadata endpoint on a cloud machine. Going by the code, the caller
gets back an embedding or an error message, so the impact depends on what those services do
with a plain GET and what the error text gives away.

## What 0.0.78 changes
[PR #713](https://github.com/michaelfeil/infinity/pull/713) adds a block list for media
URLs. The default list covers localhost, loopback, private and link-local ranges, shared
address space and multicast. The check runs on the URL, on the addresses the connection
actually resolves to, and on every redirect target. Redirects are followed by hand, up to
ten, and the session no longer reads proxy settings or `.netrc`. `INFINITY_MEDIA_URL_BLACKLIST` configures the list; setting it
replaces the defaults, and an empty value turns block-list matching off.

Two follow-ups shipped in the same release. [#714](https://github.com/michaelfeil/infinity/pull/714)
reduced the rule syntax to host names, globs and address ranges.
[#715](https://github.com/michaelfeil/infinity/pull/715) caps each download at 128 MiB
(`INFINITY_MEDIA_MAX_DOWNLOAD_BYTES`) and rejects compressed responses. The
[0.0.78 release notes](https://github.com/michaelfeil/infinity/releases/tag/0.0.78) list all
three.

Two things stay as they were. The API key still defaults to empty, so a default install
still accepts unauthenticated requests. And a block list refuses the ranges it names and
allows the rest. The [earlier post on redirects]({% post_url 2026-10-17-your-ssrf-allowlist-forgets-the-redirect %})
argues for allowing only globally routable addresses instead; check the default list against
your own network plan.

## What to do
1. **Upgrade** to 0.0.78 or later: `pip install 'infinity_emb[all]==0.0.78'`.
2. **Set an API key** with `INFINITY_API_KEY`, and do not expose the port to networks you do
   not trust.
3. **Limit egress at the network layer.** Deny outbound traffic from the serving host to
   internal ranges and the metadata endpoint, and allow only what the deployment needs. The
   in-process check is one layer; an operator can replace or empty it.
4. If you set `INFINITY_MEDIA_URL_BLACKLIST`, repeat the default entries in your value,
   because yours replaces them. The
   [deployment docs](https://github.com/michaelfeil/infinity/blob/0.0.78/docs/docs/deploy.md#remote-image-and-audio-urls-infinity-v1)
   list them.

## Timeline (UTC)
- **17 Oct 2026:** reported privately to the maintainer.
- **19 Oct, 05:23:** the fix is merged in #713, about 31 hours after the report.
- **19 Oct, 16:52:** 0.0.78 is published.
- **19 Oct:** the project has no published
  [security advisory](https://github.com/michaelfeil/infinity/security/advisories) for this
  issue. I am requesting a CVE ID and will add it here when it is assigned.

Thanks to the maintainer, [@michaelfeil](https://github.com/michaelfeil), for the quick fix
and a release within two days of the report.

Same thread as [the gate]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %}):
the default decides the bug. Here the defaults are an open port with no key unless one is
set, and, before 0.0.78, a fetcher that went wherever it was pointed.
