---
layout: post
title: "The cheap gate I run before reading an inference server's code"
date: 2026-10-17
---
Model inference servers have a recurring weakness, and it is almost always the same shape:
the server fetches a URL the client handed it, with no authentication and no check on
where the URL points.

A multimodal model wants an image, so the API lets you pass `image_url` instead of raw
bytes, and the server downloads it. A model server wants weights, so it downloads them
from a path or a bucket you name. Each of those is the server making an outbound request
on your behalf. If the server has no auth by default and never checks the destination, an
attacker who can reach it can make it fetch `http://169.254.169.254/` (the cloud metadata
endpoint), a loopback admin port, or an internal service it should never touch. That is
server-side request forgery, and in this corner of the ecosystem it is common.

The hard part is not finding a candidate. It is not burning a day on one that was already
fixed, already reported, or safe on purpose. So before I read a single line of a project's
code, I run a gate.

## Check the channel and the duplicates first
Two cheap questions decide whether the work is even worth doing, and both answer from
public metadata in about a second:

1. **Is there a way to report privately?** GitHub Private Vulnerability Reporting turned
   on, or a `SECURITY.md` with an address. An archived, retired, or "not for production"
   project has no channel at all — there is nowhere for a fix or an advisory to come from,
   so however real the bug is, stop.
2. **Has someone already found it?** Published advisories, open issues, and the CVE
   databases. A project that already has several advisories in the exact class you are
   chasing has usually hardened that surface, not left it open. The unmined bet is a
   project where the class is plausible but the advisory count is zero.

I only clone the repository if the gate passes. A short script answers both questions with
no code reading, and it has saved me far more time than it cost to write.

## Read the defaults, not the README
Once the code is open, the finding lives or dies on one question: what happens with the
default configuration, before the operator changes anything?

- **Is auth actually off by default?** It is easy to read `if api_key:` and assume a key is
  required. Follow it: more than once the key defaulted to empty, and the check was skipped
  entirely. The opposite also happens — a guard that looks absent is on by default — and
  that kills a "finding" before you waste a disclosure on it.
- **Does the guard survive a redirect?** A destination allowlist that checks the first URL
  but follows redirects without re-checking is not a guard. An allowed host that answers
  with a `302` to the metadata endpoint walks straight through it.
- **Prove it the slow way.** When a path looks like traversal, test `%2F` against `%252F`
  on a real server, not an abstraction of one — the decoding layers do not always agree,
  and the difference is the whole bug.

## Disclose the boring way
Private channel. One finding per report, so each stands on its own. Enough detail to
reproduce, read out of the source — never a proof-of-concept fired at someone else's
running system. Then wait, and let the maintainer publish on their schedule.

One report that is already public, because the project uses public issues as its disclosure
channel, is an unauthenticated SSRF in [SeldonIO's MLServer](https://github.com/SeldonIO/MLServer/issues/2622):
the HuggingFace runtime fetches a media URL from the request with no destination check and
no auth in front of it. Same shape as above. The rest are in coordinated disclosure, so I
will not name them until the maintainers do.

If you build one of these servers, the single highest-value change is to decide, on purpose,
what your server is allowed to connect to — and to re-decide it after every redirect.
