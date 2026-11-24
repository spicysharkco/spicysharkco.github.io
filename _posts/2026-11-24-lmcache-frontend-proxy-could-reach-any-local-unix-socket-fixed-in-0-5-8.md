---
layout: post
title: "LMCache frontend proxy could reach any local UNIX socket: fixed in 0.5.8"
date: 2026-11-24
---
The LMCache frontend has a proxy route that forwards requests to registered nodes. From
0.4.7 up to 0.5.8, it checked TCP targets against its node registry, but it passed a UNIX
socket path from the caller straight to its HTTP client without checking it. Anyone who
could reach the frontend's port could send requests to any UNIX socket on the host.
Version 0.5.8 fixes it. The advisory is
[GHSA-hhqh-8jpj-27rf](https://github.com/LMCache/LMCache/security/advisories/GHSA-hhqh-8jpj-27rf),
published on 23 Nov 2026. The advisory has no CVE ID yet; I will add it here if one is
assigned.

I found the same bug independently. My report was a duplicate of the report this advisory
publishes, written by Ali AlMousa, and the advisory credits me as a finder. The facts below come
from the published advisory.

## What is affected
- **Product:** the LMCache frontend, `lmcache/lmcache_frontend/app.py`, its `/proxy` route.
- **Affected:** lmcache 0.4.7 up to, but not including, 0.5.8.
- **Fixed:** 0.5.8.
- **Severity:** critical, CVSS 3.1 score 9.8
  (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H`), CWE-918 (server-side request forgery).
- **Preconditions:** the frontend is running, and the caller can reach its port. It binds
  `0.0.0.0` by default. A deployment that never starts the frontend is not affected. What
  the caller gets then depends on the configuration:
  - Environment disclosure also needs `internal_api_server_enabled`. The advisory grades
    this case 8.5, High.
  - The 9.8 rating is for code execution, which also needs `run_script_api_enabled`.

## Where it was
For a TCP target, the proxy looks the host and port up in the node registry and returns 403
for anything not registered. Its docstring says this keeps the endpoint from being used as
an open relay. Socket paths took a separate branch. That branch passed the caller's path to
`httpx.AsyncHTTPTransport(uds=...)` with no validation. The docstring says socket paths are
accepted as-is because they are local endpoints. That is true of where the connection goes,
not of who asks for it. In 0.5.6, the version the advisory tested, the frontend app had no
authentication. The caller chose the method, path, headers and body, and got the response
back.

## Why it matters
LMCache's production frontend documentation puts its internal API server on UNIX sockets,
and that server has no authentication of its own. Through the proxy, the advisory shows two
results:
- `GET /env` returned the whole environment of the serving process. In the advisory's test,
  the response included a planted Hugging Face token. Only `LMCACHE_HTTP_API_KEY` is
  redacted.
- Where `run_script_api_enabled` is set, as the example in the production frontend
  documentation sets it, `POST /run_script` ran the caller's code as the serving user. The
  advisory adds that the sandbox of that endpoint does not confine the code.

The advisory also states its limits. The bug is not an arbitrary file read: a path that is
not a listening socket returns an error. The registry check on the TCP branch is sound.
Reaching a Docker socket is an inference from the same primitive, not a demonstrated result.

## What 0.5.8 changes
The fix is [#6497](https://github.com/LMCache/LMCache/pull/6497). Per the advisory, the proxy
now forwards only to UNIX sockets declared at startup (`--nodes`, `--config` or
`--uds-allowlist`). It returns 403 for any other socket path, with or without an API key.
Declared sockets stay reachable without authentication unless a frontend API key is set
(`--api-key-file`, `LMCACHE_API_KEY` or `LMCACHE_API_KEY_FILE`, available since 0.5.7). The
frontend still binds `0.0.0.0` by default.

## What to do
1. **Upgrade** lmcache to 0.5.8 or later.
2. **Set a frontend API key.** After the upgrade, the declared sockets are still open to any
   caller who reaches the frontend, unless a key is configured.
3. **Limit who can reach the frontend.** It binds all interfaces by default. Restrict its
   port to callers you trust.
4. **Check `run_script_api_enabled`.** It is off by default. If you turned it on from the
   production example, decide whether you need it.
5. **Rotate the secrets in the serving process's environment** if an affected frontend was
   reachable while the internal API server was enabled. If `run_script_api_enabled` was
   also set, assume code may have run as the serving user.

Thanks to Ali AlMousa for the report the advisory publishes, and to the LMCache maintainers
for the fix and the advisory.
