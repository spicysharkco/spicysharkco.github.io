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
import requests
from urllib.parse import urlparse

def fetch(url):
    if urlparse(url).hostname not in ALLOWED_HOSTS:
        raise ValueError("host not allowed")
    return requests.get(url, timeout=5)   # allow_redirects=True by default
```

The check runs once, on the URL the client handed you. Whether the client then follows
redirects depends on the client. These follow redirects by default: `requests` (every helper
except `head`), `urllib.request`, `aiohttp` and Go's `net/http`. These do not:
`requests.head`, `httpx` (since 0.20) and `curl` without `-L`. Check yours. Where it does
follow, the `Location` is chosen by the server that answered, not by you, so the request that
actually leaves the box is not the URL you validated. Trusting hop zero says nothing about
hop one.

Two ways that becomes a bug:

- **An allowed host with an open redirect.** A trusted domain that has an open redirect
  will bounce you to an arbitrary URL if asked the right way. Your allowlist said yes to
  the domain; the domain said `302 Location: http://169.254.169.254/`.
- **A name that points inward.** You checked the hostname string, but DNS resolves it to a
  loopback or private address — or resolves twice, once for your check and once for the
  connect, to different answers. The string passed; the socket went elsewhere.

## Validate what you connect to, not what you were told
Check the destination on every hop, the first one included, and follow redirects yourself:

```python
import requests
from urllib.parse import urljoin

MAX_HOPS = 5

def fetch(url):
    for _ in range(MAX_HOPS + 1):
        check_destination(url)                              # hop zero and every hop after
        resp = requests.get(url, allow_redirects=False, timeout=5)
        if not resp.is_redirect:
            return resp
        url = urljoin(resp.url, resp.headers["Location"])   # Location may be relative
    raise ValueError("too many redirects")                  # fail closed past the cap
```

The first URL goes through the same check as every redirect target. A relative `Location`
is resolved against the URL that returned it before it is checked. After five redirects
the loop gives up instead of returning whatever it has.

`check_destination` is where the work is. The allowlist says which names you trust; the
address check says what those names may resolve to, and it should allow only globally
routable addresses rather than subtract a few bad ones:

```python
import ipaddress
import socket
from urllib.parse import urlparse

NAT64 = ipaddress.ip_network("64:ff9b::/96")
GLOBAL_V6 = ipaddress.ip_network("2000::/3")
# Not global, or an IPv4 address in disguise. Some Python releases call these global.
REJECT = [ipaddress.ip_network(n) for n in (
    "192.0.0.0/24",      # IETF protocol assignments
    "192.88.99.0/24",    # 6to4 relay anycast
    "2001::/23",         # IETF protocol assignments, including Teredo (2001::/32)
    "2002::/16",         # 6to4
    "3fff::/20",         # documentation
)]

def public(addr):
    ip = ipaddress.ip_address(addr)
    if ip.version == 6:
        if ip.ipv4_mapped is not None:             # ::ffff:127.0.0.1
            ip = ip.ipv4_mapped
        elif ip in NAT64:                          # 64:ff9b::7f00:1
            ip = ipaddress.IPv4Address(int(ip) & 0xFFFFFFFF)
        elif ip not in GLOBAL_V6:                  # ::1, fc00::/7, fe80::/10, ...
            return False
    return ip.is_global and not ip.is_multicast and not any(ip in net for net in REJECT)

def check_destination(url):
    # Before anything else: refuse what a second URL parser could read differently.
    if any(c == "\\" or c.isspace() or not c.isprintable() for c in url):
        raise ValueError("destination not allowed")
    parts = urlparse(url)
    if parts.username is not None or parts.password is not None or "@" in parts.netloc:
        raise ValueError("destination not allowed")      # no userinfo, even an empty one
    if parts.scheme not in ("http", "https") or parts.hostname not in ALLOWED_HOSTS:
        raise ValueError("destination not allowed")
    port = parts.port or (443 if parts.scheme == "https" else 80)
    infos = socket.getaddrinfo(parts.hostname, port, type=socket.SOCK_STREAM)
    if not all(public(info[4][0]) for info in infos):
        raise ValueError("destination resolves to a non-public address")
    return [info[4][0] for info in infos]
```

- **Refuse URLs that two parsers could read differently.** `urlparse` reads
  `http://evil.test:8080\@allowed.test/` as host `allowed.test`, because it counts
  everything before the `@` as userinfo. urllib3 2.6, which `requests` uses, ends the
  authority at the backslash and connects to `evil.test`. So `check_destination` first
  refuses a backslash, whitespace, control characters and any userinfo. The `requests` loop
  is only as safe as the agreement between the two URL parsers; `fetch_pinned` below avoids
  that gap because it connects to the host and port it parsed itself, and no second parser
  sees the URL.
- **Check every address, not the string.** `getaddrinfo` can return several, and one inward
  address is enough to refuse. In this code the allowlist runs before resolution, so a
  numeric spelling of the loopback address never reaches `getaddrinfo`. The spellings
  matter if you use a deny-list instead, or accept raw IP input: a resolver can read
  `2130706433` and `0x7f.1` as `127.0.0.1`, and parsers disagree about `0177.0.0.1`:
  `inet_aton`-style parsers read it as octal `127.0.0.1`, while older releases of Python's
  `ipaddress` and Go's `net.ParseIP` read it as decimal `177.0.0.1` (CVE-2021-29921,
  CVE-2021-29923). Judge the resolved addresses, not the string.
- **Unwrap embedded IPv4 first.** `::ffff:127.0.0.1` (IPv4-mapped) and `64:ff9b::7f00:1`
  (NAT64) carry an IPv4 address inside an IPv6 one. The NAT64 prefix is registered as
  globally reachable, so `is_global` alone lets the IPv6 form through; judge the IPv4
  address inside it. 6to4 (`2002::/16`) and Teredo (`2001::/32`) embed IPv4 addresses too
  and are rejected outright.
- **Everything not global is out.** That covers `0.0.0.0/8`, carrier-grade NAT space
  (RFC 6598), loopback, link-local, RFC 1918, `::1`, `fc00::/7` and `fe80::/10`. Python's
  `is_global` follows the IANA special-purpose registries from Python 3.13 (gh-113171),
  and from the patch releases 3.12.4, 3.11.10, 3.10.15 and 3.9.20 (CVE-2024-4032); earlier
  releases get some ranges wrong, which is why `REJECT` lists them explicitly and the
  check gives the same answers on 3.9 and later. `is_global` is also true for multicast
  addresses, so the check excludes those, and the `2000::/3` test keeps IPv6 to global
  unicast, which also leaves out local-use NAT64 (`64:ff9b:1::/48`).

One gap is left. You resolve to check, then `requests` resolves again to connect, so a name
that answers differently the second time (DNS rebinding) slips past. Close it by connecting
to the address you checked, while keeping the name for the `Host` header, the TLS server
name and the certificate check. With the standard library, one pinned hop is `fetch_pinned`,
and the loop calls it instead of `requests.get`. It reuses `MAX_HOPS` and
`check_destination` from above:

```python
import http.client
import socket
import ssl
from urllib.parse import urljoin, urlsplit

MAX_BYTES = 1_000_000


class Pinned:
    """Dial pinned_ip; keep the name for the Host header.

    For HTTPS the name is also used for SNI and the certificate check.
    """
    def __init__(self, host, pinned_ip, **kw):
        super().__init__(host, **kw)
        self.pinned_ip = pinned_ip

    def dial(self):
        return socket.create_connection((self.pinned_ip, self.port), self.timeout)


class PinnedHTTP(Pinned, http.client.HTTPConnection):
    def connect(self):
        self.sock = self.dial()


class PinnedHTTPS(Pinned, http.client.HTTPSConnection):
    def connect(self):
        ctx = ssl.create_default_context()
        self.sock = ctx.wrap_socket(self.dial(), server_hostname=self.host)


def fetch_pinned(url):
    """One GET, no redirects followed. Returns (status, Location or None, body)."""
    parts = urlsplit(url)
    ip = check_destination(url)[0]              # raises unless every address is public
    conn_class = PinnedHTTPS if parts.scheme == "https" else PinnedHTTP
    conn = conn_class(parts.hostname, ip, port=parts.port, timeout=5)
    try:
        conn.request("GET", (parts.path or "/") + ("?" + parts.query if parts.query else ""))
        resp = conn.getresponse()
        body = resp.read(MAX_BYTES + 1)             # one byte over the cap is enough to refuse
        if len(body) > MAX_BYTES:
            raise ValueError("response too large")
        return resp.status, resp.getheader("Location"), body
    finally:
        conn.close()


def fetch(url):
    for _ in range(MAX_HOPS + 1):
        # fetch_pinned checks the destination, then dials that address.
        status, location, body = fetch_pinned(url)
        if status not in (301, 302, 303, 307, 308):
            return status, body
        if not location:
            raise ValueError("redirect without a Location")
        url = urljoin(url, location)                # Location may be relative
    raise ValueError("too many redirects")
```

`fetch_pinned` runs the destination check itself and dials the first address it approved,
so a name is never resolved twice. It uses only that first address and does not try the
others, so a name whose first address is down fails instead of falling back. It reads at
most `MAX_BYTES + 1` bytes and refuses a larger body. `timeout=5` limits each socket
operation, not the whole fetch, so a server that keeps sending slowly can still hold the
call open. Nothing in `http.client` follows redirects, so the loop is the only place a hop
can happen. A pin only covers connections you open: this code ignores `HTTP_PROXY` and
`HTTPS_PROXY`, but a client that honours them, such as `requests`, hands the request to the
proxy, the proxy resolves the name, and your pin is bypassed. Enforce the rule at the proxy
in that case. Either way, default egress to deny, so a destination has to be chosen on
purpose.

## How others solve it
The durable pattern is a small resolver layer under the HTTP client: resolve, check the
address against what is globally routable, pin it, repeat on every redirect. Some stacks
let you disable redirects and hand you each hop to approve. The common move is the same —
the decision shifts from the hostname you were given to the address you are about to open.

Same thread as the rest of this trio: [the gate]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %}) finds
the endpoint, and the default decides the bug. Here the default can be "follow redirects,"
and a check that fires once, on hop zero, is a check the network routes around.

*Updated 2026-10-18: The fixed loop now checks the first URL as well as every redirect
target, resolves relative redirects and caps the hops. Added a check that refuses URLs two
parsers can read as different hosts, replaced the deny-list advice with an allowlist of
globally routable addresses that unwraps IPv4-mapped and NAT64 forms, and added a pinned
connection with a complete `fetch_pinned`, a DNS rebinding fix and a proxy caveat.
Corrected which clients follow redirects by default, and removed unsourced claims about how
many clients and domains behave this way. Every change is in the
[file history](https://github.com/spicysharkco/spicysharkco.github.io/commits/main/_posts/2026-10-17-your-ssrf-allowlist-forgets-the-redirect.md).*
