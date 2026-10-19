---
layout: page
title: Security
permalink: /security/
---

I report vulnerabilities I find in open-source model-serving and inference software to the maintainers, through the project's private channel when it has one.

## How I disclose

- **Private first.** If the project has GitHub private vulnerability reporting, I file there. Otherwise I follow its SECURITY.md, and I also look for a policy in the vendor's main repository.
- **Public only when there is no private route.** A report filed to a public issue tracker is public from day one. It names the file and line. It says what the default configuration does. It leaves out any request body or script that works against a deployment.
- **Verified from source.** I verify by reading the source. Any reproduction runs locally against a throwaway build, never against a live system.
- **One bug, one report.** Distinct root causes get separate reports, so each fix can be tracked and credited on its own.
- **On fix, or 45 days.** I publish details once a fix ships or the maintainer publishes an advisory. If neither happens within 45 days of the report, I publish the affected file, the bug class and the mitigation, not a working request. A maintainer can ask for more time with a reason. If I agree, I post the new date in the report thread. For a report filed before 2026-10-18, the 45 days run from a notice of this policy that I sent the maintainer. The notice states the date.

## Disclosures

An entry appears here after a fix ships, after the maintainer publishes an advisory, or after the 45 days pass. A report that was public from the start appears at once.

| Reported | Project | Summary | Status | Link |
|---|---|---|---|---|
| 2026-10-17 | michaelfeil/infinity | Server-side request forgery: the server fetched images and audio from any URL a client sent, without checking where the URL pointed | fixed in 0.0.78; CVE ID not yet assigned | [write-up]({% post_url 2026-10-19-infinity-media-url-fetch-ssrf-fixed-in-0-0-78 %}) |
| 2026-10-17 | SeldonIO/MLServer | Server-side request forgery: the Hugging Face runtime passes a client-supplied media string to a `transformers` pipeline, which fetches it as a URL | open (public issue) | [issue #2622](https://github.com/SeldonIO/MLServer/issues/2622) |

## Private reports

As of 2026-10-19, eight other reports are filed privately with their maintainers. I name none of them until a fix ships, the maintainer publishes an advisory, or the 45 days pass.

## Method notes

- [The cheap gate I run before reading an inference server's code]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %})
- [Loading model weights can run a stranger's code]({% post_url 2026-10-17-loading-model-weights-can-run-a-strangers-code %})
- [Your SSRF allowlist forgets the redirect]({% post_url 2026-10-17-your-ssrf-allowlist-forgets-the-redirect %})

## Report a vulnerability to me

To report a security problem in this site or in my GitHub repositories, email spicysharkco@gmail.com.

— spicysharkco
