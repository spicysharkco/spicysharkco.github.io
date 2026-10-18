---
layout: post
title: "The cheap gate I run before reading an inference server's code"
date: 2026-10-17
---
One shape is worth checking first in a model inference server: the server fetches a URL
the client handed it, with no authentication and no check on where the URL points.

A multimodal model wants an image, so the API lets you pass `image_url` instead of raw
bytes, and the server downloads it. A model server wants weights, so it downloads them
from a path or a bucket you name. Each of those is the server making an outbound request
on your behalf. If the server has no auth by default and never checks the destination, an
attacker who can reach it can make it fetch `http://169.254.169.254/` (the cloud metadata
endpoint), a loopback admin port, or an internal service it should never touch. That is
server-side request forgery (SSRF).

The hard part is not finding a candidate. It is not burning a day on one that was already
fixed, already reported, or safe on purpose. So before I read a single line of a project's
code, I run a gate.

## Check the channel and the duplicates first
Two cheap questions decide whether the work is even worth doing, and both answer from
public metadata, without reading any code:

1. **Is there a way to report privately?** GitHub Private Vulnerability Reporting turned
   on, or a `SECURITY.md` with an address. An archived, retired, or "not for production"
   project has no channel at all — there is nowhere for a fix or an advisory to come from,
   so however real the bug is, stop. A live project that only lacks a private channel is a
   harder call: its public tracker is then the only place a report can go, and everything
   posted there is public from the first minute, so the report should name the file and
   line, say what the default configuration does and how the input reaches it, and leave
   out a request body or a script that works against a deployment.
2. **Has someone already found it?** Published advisories, open issues, and CVE ids. A
   project with advisories already published in the class you are chasing may have fixed
   that surface, so read what they fixed before assuming it did not. A project where the
   class is plausible but the advisory count is zero is worth a read, though zero
   advisories only means none were published.

I only clone the repository if the gate passes. A few lookups answer both questions with
no code reading (replace `OWNER`, `REPO`, `PKG`, and `FLAGSHIP`, the vendor's main
repository):

```sh
# 1. Is the repository archived? Prints true or false.
gh api repos/OWNER/REPO --jq .archived

# 1. Is private vulnerability reporting switched on? Prints true or false.
gh api repos/OWNER/REPO/private-vulnerability-reporting --jq .enabled

# 1. Is there a security policy in the repo, in the owner's default .github repo, or in
#    the vendor's main repo? Each path is tried on the default branch, then on master and
#    main, because a policy can sit on a branch that is not the default.
for r in REPO .github FLAGSHIP; do
  for p in SECURITY.md .github/SECURITY.md docs/SECURITY.md; do
    for ref in "" master main; do
      gh api "repos/OWNER/$r/contents/$p${ref:+?ref=$ref}" --silent 2>/dev/null \
        && echo "policy: OWNER/$r/$p ref=${ref:-default}"
    done
  done
done

# 2. Which advisories are already published for the repository?
gh api repos/OWNER/REPO/security-advisories --jq '.[].summary'

# 2. Which advisories in GitHub's database, which carries CVE ids, name the package?
#    PKG is the package name, which can differ from the repository name.
gh api "advisories?affects=PKG" --jq '.[] | .ghsa_id + " " + (.cve_id // "-") + " " + .summary'

# 2. Which issues already mention the class? Issues use both spellings.
for q in ssrf "server-side request forgery"; do
  gh search issues "$q" --repo OWNER/REPO --json number,title,state --limit 5
done
```

The archived and private-reporting calls print `true` or `false` (the raw
private-reporting response is `{"enabled": false}`). A missing file is a `404`, which makes
`gh api` exit non-zero, so only a policy that exists prints a line. The contents API is
case-sensitive, and the loop tries only the default branch, `master` and `main`, so a
`security.md`, a policy under another name, or one on any other branch is missed: a miss
means keep looking, not that no policy exists. The lookups cover GitHub's advisory
database and issue tracker. A CVE that lives only in another database needs its own
search.

## Read the defaults, not the README
Once the code is open, the finding lives or dies on one question: what happens with the
default configuration, before the operator changes anything?

- **Is auth actually off by default?** It is easy to read `if api_key:` and assume a key is
  required. Follow it: the key can default to empty, and then the check is skipped
  entirely. The opposite also happens — a guard that looks absent is on by default — and
  that kills a "finding" before you waste a disclosure on it.
- **Does the guard survive a redirect?** A destination allowlist that checks the first URL
  but follows redirects without re-checking is not a guard. An allowed host that answers
  with a `302` to the metadata endpoint walks straight through it.
- **Prove it the slow way.** When a path looks like traversal, test every encoding of the
  path separator end to end against a running instance that you run yourself, not an
  abstraction of one — the decoding layers do not always agree, and the difference is the
  whole bug.

## Disclose the boring way
A private channel when there is one. One finding per report, so each stands on its own.
Enough detail to reproduce, read out of the source — never a proof-of-concept fired at
someone else's running system. Then wait. I publish details once a fix ships or the
maintainer publishes an advisory. If neither happens within 45 days of the report (for a
report filed before 2026-10-18, 45 days from a dated notice I send first), I publish the
affected code and the mitigation, not a working request. A maintainer can ask for more
time. Private reports stay private until then.

One report was public from the start. When I checked,
[SeldonIO's MLServer](https://github.com/SeldonIO/MLServer/issues/2622) had no
`SECURITY.md` and private vulnerability reporting was switched off, so its own tracker
offered no private way to send it. The vendor's main repository, `SeldonIO/seldon-core`,
has a `SECURITY.md` on its `master` branch, which is not its default branch. Its
reporting section reads:

> If you identify a vulnerability, if a public CVE the best way to report it is by opening
> an issue with the type "bug", the discussion can then take place on the ticket around
> next steps (ie updating library, reaching out to 3rd party projects, etc).

The issue cites that file, but the sentence is conditional on a public CVE, and the file
names no private route for MLServer, which is a different repository. Filing a public
issue was a judgment call, not a documented route. That is why the lookup above also
checks the vendor's main repository and the `master` and `main` branches. The report is an
unauthenticated SSRF: the Hugging Face runtime passes a client-supplied media string to a
`transformers` pipeline, the pipeline fetches it as a URL, and MLServer puts no
destination check and, by default, no authentication in front of it. Same shape as above.

If you build one of these servers, the change I would make first is to decide, on purpose,
what your server is allowed to connect to — and to re-decide it after every redirect.

*Updated 2026-10-18: Replaced the open-ended wait with a stated disclosure policy, and
dropped the sentence about the other reports being in coordinated disclosure. Removed
unsourced claims, including how common this weakness is and that projects with advisories
have usually hardened that surface, and softened "the single highest-value change".
Replaced the gate's script with the lookups it relies on, added a rule for reports to a
public tracker, and corrected the MLServer account to quote the vendor's `SECURITY.md`
exactly. Every change is in the
[file history](https://github.com/spicysharkco/spicysharkco.github.io/commits/main/_posts/2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code.md).*
