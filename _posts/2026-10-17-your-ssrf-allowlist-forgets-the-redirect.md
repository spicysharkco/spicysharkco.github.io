---
layout: post
title: "Your SSRF allowlist forgets the redirect"
date: 2026-10-17
---
You added a destination check to the URL fetcher, so it only talks to hosts you trust.
Good. Then one of those trusted hosts answers `302` to an address you would never allow,
your client follows it without asking, and the request you were guarding lands exactly
where you were guarding against.

## The guard that only sees hop zero
Most allowlists look like this:

```python
if urlparse(url).hostname not in ALLOWED_HOSTS:
    raise ValueError("host not allowed")
return requests.get(url, timeout=5)   # allow_redirects=True by default
```

The check runs once, on the URL the client handed you. But `requests` — like almost every
HTTP client — follows redirects by default, and the `Location` it follows is chosen by the
server that answered, not by you. The request that actually leaves the box is not the URL
you validated. Trusting hop zero says nothing about hop one.

Two ways that becomes a bug:

- **An allowed host with an open redirect.** Plenty of trusted domains will bounce you to
  an arbitrary URL if asked the right way. Your allowlist said yes to the domain; the
  domain said `302 Location: http://169.254.169.254/`.
- **A name that points inward.** You checked the hostname string, but DNS resolves it to a
  loopback or private address — or resolves twice, once for your check and once for the
  connect, to different answers. The string passed; the socket went elsewhere.

## Validate what you connect to, not what you were told
```python
resp = requests.get(url, allow_redirects=False, timeout=5)
while resp.is_redirect:
    target = resp.headers["location"]
    if not destination_allowed(target):   # re-resolve, reject link-local/loopback/private
        raise ValueError("redirect blocked")
    resp = requests.get(target, allow_redirects=False, timeout=5)
```

That closes the redirect hole, but the hostname string is still the wrong thing to trust.
The robust version resolves the name, rejects link-local, loopback and RFC1918 addresses,
then connects to that pinned IP — so a second lookup cannot swap in an internal one after
the check. Re-run it on every hop, and default egress to deny so a destination has to be
chosen on purpose.

## How others solve it
The durable pattern is a small resolver layer under the HTTP client: resolve, check the
address against a deny list, pin it, repeat on every redirect. Some stacks let you disable
redirects and hand you each hop to approve. The common move is the same — the decision
shifts from the hostname you were given to the address you are about to open.

Same thread as the rest of this trio: [the gate]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %}) finds
the endpoint, and the default decides the bug. Here the default is "follow redirects," and
a check that fires once, on hop zero, is a check the network routes around.
