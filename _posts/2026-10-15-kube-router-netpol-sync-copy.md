---
layout: post
title: "kube-router: a dead check that searched the filter-table buffer once per pod"
date: 2026-10-15
project: kube-router
pr: https://github.com/cloudnativelabs/kube-router/pull/2192
status: merged
tags: [kubernetes, performance, networking, go]
---

kube-router's network-policy controller rebuilds a node's iptables rules in one in-memory buffer on every sync. Until v2.12.0, it checked each local pod's log rule against that buffer before writing it. It did that by copying the whole filter-table buffer to a string and searching it. The check could never be true, so every search read to the end. The cost of one sync was pods times table size, and the table itself grows with pods.

I removed the check in [PR #2192](https://github.com/cloudnativelabs/kube-router/pull/2192), merged 2026-10-12 and released in v2.12.0 on 2026-11-02. This post measures what the check cost across node sizes. It shows where the time went: the search, not the copy I first blamed. It also explains why deleting the check was safe.

| Case (one IP family, one call of `syncPodFirewallChains`) | Before | After |
|---|---|---|
| 80 pods, 1 MiB table | 273 ms, 86.6 MiB allocated | 0.92 ms, 0.20 MiB |
| 110 pods, table grows with pods | 190 ms, 58.6 MiB | 1.19 ms, 0.25 MiB |
| 250 pods, table grows with pods | 723 ms, 221 MiB | 2.72 ms, 0.64 MiB |
| 80 pods, 4 MiB table | 1.02 s, 338 MiB | 1.21 ms, 0.21 MiB |

The setup and the limits are below. "After" times include the benchmark's own refill of the table, at most 0.36 ms in these rows.

## What network-policy sync does

The controller turns Kubernetes NetworkPolicy objects into iptables rules on every node. k3s embeds it: the k3s docs say "K3s includes an embedded network policy controller. The underlying implementation is kube-router's netpol controller library".

It works by full sync. `fullPolicySync` does roughly this:

1. Run `iptables-save` for the filter table into a `bytes.Buffer`, one per IP family.
2. Append the policy chains (`KUBE-NWPLCY-*`) and, for every pod on the node, a firewall chain (`KUBE-POD-FW-*`) with its rules.
3. Strip stale rules from the buffer.
4. Hand the buffer to `iptables-restore`.

How often that runs matters as much as what one run costs. The controller's pod informer watches every pod in the cluster, not only the pods on its node. These events each call `RequestFullSync`:

- a new pod that has an IP, is not host-network and has not finished;
- any pod delete;
- a pod update that changes its phase, IP, host IP or labels;
- namespace and NetworkPolicy changes;
- a timer every `--iptables-sync-period` (5 minutes by default).

Requests go through a channel with room for one, and syncs run one at a time. A burst of events therefore collapses into one more sync. If events keep arriving faster than a sync finishes, though, the next sync starts as soon as the last one ends.

So whatever one sync costs, every node pays it each time most pods change anywhere in the cluster.

## The check

Inside `syncPodFirewallChains`, for every local pod and each IP family the pod has an address in, this ran before the pod's log rule was written:

```go
// This used to be AppendUnique when we were using iptables directly, this checks to make sure we didn't drop
// unmarked for this chain already
if strings.Contains(filterTableRules.String(), strings.Join(args, " ")) {
	continue
}
```

The comment is the history. When kube-router drove iptables one command at a time, `AppendUnique` meant "add this rule unless it is already there". Once the rules were built in a buffer, the check became a search of the buffer.

## The cost model

Per pod, the check does two things:

1. `filterTableRules.String()` copies the buffer. Its body in the Go standard library is `return string(b.buf[b.off:])`, one allocation and a copy of everything in it.
2. `strings.Contains` searches the copy for the pod's 178-byte log rule. The rule is never there, so the search reads every byte.

At that point the buffer holds the whole `iptables-save` output plus every rule this sync has written so far. Work per sync is therefore about pods times table bytes.

The two factors are not independent. `iptables-save` prints the chains the previous sync wrote. In these benchmarks one sync writes about 1.7 KB of pod firewall chains per pod (17,210 bytes for 10 pods, 191,310 for 110). Real sizes depend on name lengths, because the rules carry the namespace and pod name. Part of the table grows with the pod count, so part of the cost grows with the pod count squared.

## Measurements

The "after" code is kube-router at the merge commit, `ecf5008a`. The "before" code is the same tree with the five deleted lines put back. The measurements come from scratch benchmarks built on the PR's test helpers and not sent upstream ([source](https://github.com/spicysharkco/spicysharkco.github.io/tree/main/_code/kube-router-2192)). Each one builds N pods in one IP family and fills the filter buffer with synthetic policy rules to a given size. Then it times `syncPodFirewallChains`:

```go
for b.Loop() {
	buf.Reset()
	buf.Write(existing) // the table as iptables-save left it
	npc.syncPodFirewallChains(nil, "bench")
}
```

Before and after ran alternately for six rounds on Linux, on an Intel Core i7-8665U (4 cores, 8 threads, 1.90 GHz) with Go 1.26.2. Tables show medians. Every before/after difference below has p = 0.002 in `benchstat`.

### More pods, table fixed at 1 MiB

| Pods | Before | After |
|---|---|---|
| 10 | 32.0 ms | 0.19 ms |
| 25 | 81.0 ms | 0.35 ms |
| 50 | 165.9 ms | 0.61 ms |
| 80 | 272.9 ms | 0.92 ms |
| 110 | 383.1 ms | 1.22 ms |
| 250 | 961.8 ms | 2.74 ms |

Before, each extra pod added about 3.9 ms. After, it adds about 11 µs. 110 is the kubelet's default `maxPods`, and 250 is a common raised limit.

### Larger table, 80 pods

| Table | Before | After | After, minus the refill |
|---|---|---|---|
| 256 KiB | 83.5 ms | 0.89 ms | 0.87 ms |
| 1 MiB | 273.7 ms | 0.92 ms | 0.84 ms |
| 4 MiB | 1.02 s | 1.21 ms | 0.85 ms |
| 16 MiB | 4.05 s | 2.86 ms | 0.99 ms |

The 1 MiB row is the same case as the 80-pod row above, from a separate run. The benchmark loop copies the table back into the buffer on every pass. I timed that refill alone (0.02 ms at 256 KiB to 1.87 ms at 16 MiB) and subtracted it in the last column.

Before, each extra MiB cost about 250 ms at 80 pods, about 3.1 ms per pod per MiB. After, the function's own time hardly moves with the table, because it no longer reads it. Its allocation does rise a little, from 0.19 MiB at 256 KiB to 0.31 MiB at 16 MiB, with the same number of allocations. That fits the buffer's one-time growth, spread over the benchmark's iterations. The old code at 4 and 16 MiB took over a second per call, so each of its six samples is a single iteration.

### Table that grows with pods

Here the table is 256 KiB of policy rules plus the pod chains a previous sync wrote for the same pods. It grows from 0.27 MiB at 10 pods to about 0.67 MiB at 250. The 256 KiB base is an assumption, and it is small: the node in the issue behind the PR had 1.2 MB at 84 pods.

| Pods | Before | After |
|---|---|---|
| 10 | 9.2 ms | 0.13 ms |
| 25 | 26.1 ms | 0.29 ms |
| 50 | 62.7 ms | 0.56 ms |
| 80 | 118.9 ms | 0.88 ms |
| 110 | 190.4 ms | 1.19 ms |
| 250 | 722.5 ms | 2.72 ms |

Going from 110 to 250 pods (2.3x) made the old code 3.8x slower and the new code 2.3x slower. The old curve bends upward; the new one is a straight line. Allocation follows the same shape. At 250 pods the old code allocated 221 MiB per call, all of it garbage once the sync ended. The new code allocated 0.64 MiB.

## Where the time went: the search, not the copy

My PR title said the problem was copying the buffer once per pod. That was the visible part. To check it, I split the old check into its two parts on the same 1 MiB table, with one pod's real log rule as the pattern:

| Operation | Time | Allocated |
|---|---|---|
| `buf.String()`, the copy | 0.24 ms | 1.0 MiB |
| `strings.Contains` on that string, the search | 2.80 ms | 0 |
| Both, as the old check did | 3.23 ms | 1.0 MiB |
| `bytes.Contains(buf.Bytes(), rule)`, search with no copy | 2.79 ms | 0 |
| `strings.Contains` with a pattern whose first byte is not in the table | 0.06 ms | 0 |

The search was about 87% of the check. The obvious "fix", searching `Bytes()` so nothing is copied, would have saved about 13%.

The search is slow because of what iptables rules look like. Go 1.26's `strings.Index`, which `Contains` calls, has a special path for long patterns. "Long" means over 63 bytes on amd64 with AVX2 (31 without) and over 32 on arm64. That path works like this:

1. Find the pattern's first byte with `IndexByte`, which is vectorized and fast.
2. Check the second byte, then the whole pattern.
3. Count each first-byte hit that is not a full match as a miss. Once misses reach `4 + i>>4`, about one per 16 bytes read, switch to Rabin-Karp for the rest of the input.

The pattern starts with `-A`, like every line of `iptables-save` output. Every option in a rule also starts with `-` (`-m`, `--comment`, `-j`, `--set-xmark`), and chain names are full of them too. My synthetic rule lines have one `-` every 9.5 bytes, which is 1.7 times the miss rate the fast path allows. The search switches 45 bytes in. Rabin-Karp then reads the rest of the megabyte with a rolling hash, one byte at a time, at about 2.7 ns per byte. The same search with a first byte that never appears in the table stays on `IndexByte` and finishes in 0.06 ms, 46 times faster.

The switch is not a Go bug. It bounds the worst case of the fast path. It does mean a substring search's speed depends on the data as well as on the length. Real tables hit the same path: the CPU profiles in the issue behind the PR list `internal/bytealg.IndexRabinKarp` as their top flat entry, at 9%.

## Why the check could go

The PR's first version kept a check but made it cheap. It replaced the buffer search with a per-family set of the rules already written in this sync:

```go
logRule := strings.Join(args, " ")
if loggedDrops[ipFamily][logRule] {
	continue
}
loggedDrops[ipFamily][logRule] = true
```

On the benchmark it gave the same result as the final version. Then the maintainer, [@aauren](https://github.com/aauren), [reviewed it](https://github.com/cloudnativelabs/kube-router/pull/2192#discussion_r4243179642). They wrote: "I would argue that we should just drop this check altogether". They added: "as far as I can tell this is dead code and never has the potential of actually coming back with a match, unless you see it differently?"

That argument holds, and the code shows why. At the time, the rule text contained:

- the pod's chain name, `KUBE-POD-FW-` plus 16 characters of base32(sha256(namespace + podName + version));
- a `<namespace>/<name> log-drop` comment.

Rules from an earlier sync cannot match. The version is derived from the sync's start time in nanoseconds, so their chain names differ. `iptables-save` also prints rules differently (`10/min` instead of `10/minute`, and a different match order).

Rules from the same sync cannot match either. The loop runs over `allLocalPods`, a map keyed by pod IP, so each pod comes through once per IP family. The informer cache holds one object per namespace and name, so no two pods write the same comment. The set guarded against a case that could not occur.

The final version deletes the check and the set, and drops the two tests that covered only them. The diff in `pod.go` is five deletions and no additions, plus a test and a benchmark.

The test, `TestSyncPodFirewallChainsDropRules`, asserts the exact rules written per chain and per family. It covers single-stack, dual-stack and colliding-name pods. It passes on the code before and after the change; I reran it on both for this update. A change that deletes a check needs a test that agrees with the old behaviour first.

Asking whether pods with colliding chain names could trigger the check turned up a separate bug. The hash joined namespace and name with no separator, so pods `a`/`bc` and `ab`/`c` got the same chain name. The old check did not fire for them, because their `log-drop` comments differ.

The maintainer filed the collision as [#2193](https://github.com/cloudnativelabs/kube-router/issues/2193). It also covers policy chains and ipset names. In their words, colliding pods can share a firewall chain, so "one pod's policy can admit the other pod's traffic". Anyone who can pick a namespace name next to an existing one can cause it, which makes it "a cross-tenant issue in multi-tenant clusters". The fix length-prefixes the hash inputs, and it is in v2.12.0 too.

## What it means on a node

The issue behind the PR ([#2191](https://github.com/cloudnativelabs/kube-router/issues/2191)) measured a node with 84 pods. Its filter buffer was 1.2 MB, with 571 policy chains. There the check took about 175 ms of CPU and 100 MB of allocation per full sync. The issue's standalone benchmark measured 180-188 ms for the check, on an AMD Threadripper PRO 5975WX with Go 1.23. At about 3.1 ms per pod per MiB, this laptop CPU would take about 300 ms at those sizes.

CPU profiles of all 8 nodes in that cluster showed `fullPolicySync` taking 26-58% of each k3s process's CPU. The check is one part of that. The PR leaves the rest alone, including `iptables-save`, `iptables-restore` and `cleanupStaleRules`. The issue describes `cleanupStaleRules` as checking every rule against every active chain, about 5,000 × 650 there.

The cost is paid per sync, and every node syncs after pod changes anywhere in the cluster. A rolling update therefore makes every node pay it repeatedly, not only the nodes where pods move. The garbage is per sync too: tens to hundreds of MiB that the Go collector has to reclaim. That happens inside the process that embeds the controller, which in k3s is the k3s process itself.

## Status

The fix is in kube-router v2.12.0, released 2026-11-02. k3s replaces kube-router with its own fork. As of 2026-11-06, its branches pin these fork versions, and all of them still have the check:

| k3s branch | Fork version | Check in `pkg/controllers/netpol/pod.go` |
|---|---|---|
| main, release-1.37, release-1.36 | `v2.11.1-k3s1` | line 116 |
| release-1.35, release-1.34 | `v2.11.1-k3s2` | line 116 |
| release-1.33, release-1.32 | `v2.6.3-k3s1` | line 97 |

k3s clusters get the fix when the fork moves to v2.12.0 or later.

## Limits

- The benchmarks time one function, `syncPodFirewallChains`, not a whole sync. `iptables-save` and `iptables-restore` are not in them.
- They cover the iptables backend only. kube-router also has an nftables backend, which I did not measure. k3s uses the iptables one.
- The tables are synthetic. They have the shape of real `iptables-save` output, rule lines that start with `-A` and are dense with options, but real sizes vary.
- One IP family. On a dual-stack node the loop runs once per family, each with its own buffer.
- This is a laptop CPU. Absolute times will differ on servers. The PR's benchmark (median of five runs) reported about 320x for 80 pods and a 1 MiB table; on this machine the same case is about 300x.
- No cluster test.

## What I take from it

- Before making a check faster, ask whether it can ever be true. The first version built a set. The maintainer asked whether the check was needed at all, which was the better answer.
- Measure the parts before naming the cause. I blamed the copy; the search was 87% of the cost.
- A loop that looks linear in pods can hide a step that is linear in buffer size. Ask what each call in the loop costs per iteration.
- Pin behaviour with a test that passes before and after, then change the code.

## How others solve it

kube-proxy in iptables mode builds its rules in buffers too, then restores them with `NoFlushTables`. I found no search of those buffers in it. It tracks the chains it wants in a set. In v1.36 that is `activeNATChains := sets.New[utiliptables.Chain]()`. It reads `iptables-save` only to list chain names with `GetChainsFromTable`, and diffs them against that set.

In large clusters, kube-proxy lists those chain names at most once per sync period. Its own comment gives the reason: `iptables-save` "can take several seconds to run on hosts with lots of iptables rules". It also has a `doFullSync` flag, so not every sync rebuilds everything.

If a check is needed, keep its state in a set. First, ask whether the check can ever be true.

If you run kube-router or k3s with many pods per node, I would like to hear what your sync times look like. kube-router logs `sync iptables took` at `-v=1`, and with metrics enabled it exports the `kube_router_controller_iptables_sync_time` histogram. Reply on [X](https://x.com/spicysharkco).

*Updated 2026-11-06: added the scaling measurements across pod counts and table sizes, the split between copy and search, the sync triggers, the k3s status and the benchmark source. The first version reported one benchmark point.*
