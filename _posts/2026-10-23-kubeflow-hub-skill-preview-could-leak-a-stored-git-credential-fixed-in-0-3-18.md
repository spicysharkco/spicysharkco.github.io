---
layout: post
title: "Kubeflow Hub skill preview could leak a stored git credential: fixed in 0.3.18"
date: 2026-10-23
---
The catalog server in kubeflow/hub has a preview endpoint for skill sources. In versions
0.3.16 and 0.3.17 it let a caller name a stored git credential and a repository URL in the
same request, and git could then send that credential to the host the caller chose. Version
0.3.18 fixes it. The advisory is
[GHSA-f88h-rcrv-66j9](https://github.com/kubeflow/hub/security/advisories/GHSA-f88h-rcrv-66j9).
The advisory has no CVE ID yet; I will add it here if one is assigned.

## What is affected
- **Product:** the catalog server in kubeflow/hub, route `POST /api/model_catalog/v1/sources/preview`
  with `assetType: skills`. The same handler is also mounted under
  `/api/model_catalog/v1alpha1/sources/preview`.
- **Affected:** 0.3.16 and 0.3.17, as the advisory lists them. I did not run each version.
  If you run 0.3.15, check your own copy: `preview.go`, `credentials.go` and
  `repo_resolver.go` under `catalog/internal/catalog/skillcatalog/` have the same git blob
  IDs on the `release/v0.3.15` branch as in the code I read (commit `09fc034`).
- **Fixed:** 0.3.18.
- **Severity:** medium (GitHub shows Moderate).
- **Preconditions:** all four have to hold.
  - The caller can reach the catalog API. The base manifests have no NetworkPolicy or
    AuthorizationPolicy for it. I make no claim about reach from outside the cluster.
  - An administrator has populated the `skill-catalog-git-credentials` Secret (optional,
    empty unless populated).
  - The caller knows or guesses a `credentialRef` name. The documented example is `github`.
  - The catalog pod can reach the host the caller picks.

## Where it was
I found this by reading the source at commit `09fc034`. `PreviewSkillSource` in
`catalog/internal/catalog/skillcatalog/preview.go` took each repository entry the caller
supplied, including its `credentialRef`, and loaded the named token from the mounted Secret
before any clone. `repo_resolver.go` checked only the transport, with no host allowlist,
and installed a `GIT_ASKPASS` helper that returned the token for every password prompt,
whichever host asked. When the chosen host answered with a Basic-auth challenge, git sent
it the token. The server had no authentication check.

A loopback-only test used a copy of the shipped askpass script, a synthetic token and host
git 2.55.0. I did not check the git version in the shipped image. For a caller-selected
HTTP host, the test server received two requests, and the second carried the synthetic
Basic credential. The same happened after a redirect from one loopback host to another. I
contacted no live or third-party system.

## Why it matters
If all four preconditions held, one populated catalog git credential went to a host the
caller picked. The endpoint also made a git smart-HTTP GET to a URL the caller picked and
returned git's error text, a limited SSRF.

## What 0.3.18 changes
The fix is [#3378](https://github.com/kubeflow/hub/pull/3378), merge commit `35167d9ac`. The
change for this advisory is the commit "fix(catalog): reject stored credentials in skill
previews".

I read the merged diff. In `PreviewSkillSource`, a repository entry with a non-empty
`credentialRef` is now rejected, before any repository is resolved, with the error
"credentialRef is not supported for skill source preview". Per the code comments, preview
now resolves anonymously and reads no credential file. A new test covers YAML and JSON
configs with the credential file missing, empty or populated, and checks that the same
error comes back and nothing is resolved.

The fix takes credential selection out of preview. The diff adds no authentication to the
endpoint and leaves `repo_resolver.go` as it was, so preview still clones the repository
URL a caller supplies, now without a credential. The
[v0.3.18 release](https://github.com/kubeflow/hub/releases/tag/v0.3.18), published about 17
hours before the advisory, lists #3378 by its title.

## What to do
1. **Upgrade** to 0.3.18 or later. The release notes do not call it a security fix, so check
   the version you run.
2. **Limit who can reach the catalog API.** Restrict ingress to the catalog Service with a
   NetworkPolicy, as the advisory suggests. Keep it after you upgrade: preview still clones
   a repository URL that the caller supplies.
3. **Check the credentials Secret.** If you never populated it, there was nothing to
   disclose this way. If you did, keep its tokens to the narrowest access the catalog needs.

## Timeline (UTC)
- **20 Oct 2026, 06:36:** reported privately through GitHub's private vulnerability reporting.
- **22 Oct:** the maintainers accepted it into a draft advisory.
- **22 Oct, 18:29:** the fix is merged in #3378, about 60 hours after the report.
- **22 Oct, 19:23:** v0.3.18 is released.
- **23 Oct, 12:19:** the advisory is published. It lists no CVE ID.

Thanks to the kubeflow/hub maintainers for the fix, and to
[@pboyd](https://github.com/pboyd), who wrote it and published the release and the
advisory.
