---
layout: post
title: "A null host in CLUSTER SLOTS: unknown-endpoint broke the go-redis cluster client"
date: 2026-10-27
project: go-redis
pr: https://github.com/redis/go-redis/pull/4131
status: merged
tags: [data, go, caching]
---

go-redis could not load a slot map from a cluster whose nodes run `cluster-preferred-endpoint-type unknown-endpoint`. `CLUSTER SLOTS` then carries a null host, and the command failed with `redis: nil`. I sent the fix ([PR #4131](https://github.com/redis/go-redis/pull/4131), merged 2026-10-26, about ten days after I opened it). The PR links no issue.

## When it happens

With that setting a node sends a null host in `CLUSTER SLOTS`. `redis.conf` says the client should reuse the host it sent the request to, with the port from the reply. Null is `$-1` on RESP2 and `_` on RESP3. `ClusterSlotsCmd` reads the host with `ReadString`, which returns `redis.Nil` for both. The command failed, the rest of the reply stayed on the pooled connection, and a `ClusterClient` could not load its slot map. I reproduced it against a real cluster node on RESP2 and RESP3. @ndyakov checked it against a fake node, and later on a local six-node cluster with `unknown-endpoint` on every node, on both protocols.

## Why the gap existed

The host read, before the fix:

```go
			ip, err := rd.ReadString()
			if err != nil {
				return err
			}
```

Any error ended the command, `Nil` included. Per @ndyakov, Redis sends the null from `addNodeToNodeReply` in `cluster.c`, in the unknown-endpoint branch, with `addReplyNull`. The sources say nothing about why `Nil` was not handled here, so I stop at what the code did.

## The fix

Two parts. In `command.go`, `Nil` is no longer an error for the host:

```diff
-			if err != nil {
+			// A null host (cluster-preferred-endpoint-type unknown-endpoint)
+			// means "reuse the origin host"; newClusterState fills it in.
+			if err != nil && err != Nil {
```

In `osscluster.go`, `newClusterState` fills an empty host from the address that returned the slot map. The new function sits next to the existing port 0 substitution:

```go
func replaceEmptyHost(nodeAddr, originHost string) string {
	nodeHost, nodePort, err := net.SplitHostPort(nodeAddr)
	if err != nil || nodeHost != "" {
		return nodeAddr
	}
	return net.JoinHostPort(originHost, nodePort)
}
```

`NodeAddress` keeps the raw `:port` value, as it does for the other substitutions. One side effect: an empty-string host, which a node sends when it does not know its own IP, used to dial `:port` on the local machine and now takes the origin host, as redis-py does. An existing loopback spec changed to expect that.

## The test

`TestNewClusterState_NullHost` loops over both encodings, `_\r\n` and `$-1\r\n`, through `readReply` and `newClusterState`. It checks the dial address, and that `NodeAddress` stays raw:

```go
if got := client.NodeAddress(); got != ":6380" {
```

Per @ndyakov it fails on master with `readReply() = redis: nil`, and with the `osscluster.go` change reverted it fails with `Addr = ":6380"`, so it covers both halves.

## What review changed

The PR has five commits. The repo's review bot and @ndyakov raised these points:

- The bot, P1: an existing spec still expected `:1234` for an empty host. Fixed in the second commit.
- The bot, P2: when the null-host entry is the origin node itself, the cached node keeps no raw `NodeAddress`. I answered that this is not specific to the empty host: `GetOrCreate` is `GetOrCreateWithNodeAddress(addr, "")`, so the seed node has none. I kept it out of this PR and offered a separate one. @ndyakov agreed it was already on master.
- @ndyakov, nit: say why `Nil` is accepted, so the guard is not removed as a typo. That is the comment in the diff above.
- The bot, P1: add a real-cluster test. I added a Ginkgo spec on the six-node cluster with `unknown-endpoint` on every node. It loads the state, runs SET and GET on two masters and forces a MOVED.
- The bot, P1, and @ndyakov: that MOVED part passes only because the test cluster is on loopback. With `unknown-endpoint` the redirect is `-MOVED <slot> :<port>`, and `:port` dials the local machine. The last commit only reworded the spec comment and `It` text to say so. Using the host of the redirecting node is a follow-up.

@ndyakov's last approval was on 2026-10-22, and they merged it on 2026-10-26.

## The lesson

- `Nil` from a typed read can be a valid value. Check what each non-error reply shape means before a guard returns on it.
- Say why an odd guard is there, in a comment, at the guard.
- Say what a test does not cover. This one stops at loopback.
- Check the sibling command. @ndyakov found that `CLUSTER SHARDS` sends `endpoint` as an empty bulk string, not NULL, so `ClusterShardsCmd` does not have this bug.

If a reader of yours returns on a null that the protocol defines as a value, reply on [X](https://x.com/spicysharkco).
