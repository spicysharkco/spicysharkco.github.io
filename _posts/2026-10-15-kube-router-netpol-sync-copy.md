---
layout: post
title: "One String() call per pod: 320x faster in one kube-router function"
date: 2026-10-15
---

I got a small change merged into kube-router ([PR #2192](https://github.com/cloudnativelabs/kube-router/pull/2192)). The code change deletes five lines; I also added a test and a benchmark. On the benchmark I wrote, one function went from 152,895,797 ns/op to 477,097 ns/op, about 320x. Bytes allocated per op fell from 90,489,076 to 205,066, which is 99.77% less.

This post covers why those five lines cost so much and why they could go.

## What network-policy sync does

kube-router's network policy controller turns Kubernetes NetworkPolicy objects into iptables rules on every node. k3s embeds it: the k3s docs say "K3s includes an embedded network policy controller. The underlying implementation is kube-router's netpol controller library". k3s pins its own fork of kube-router in `go.mod` (`v2.6.3-k3s1`), and that tag has the same line I removed. The fix is on kube-router master. The latest release I can see as of 2026-10-15, v2.11.1 from August, predates the merge. So k3s clusters get it only after that pin moves.

The controller works by full sync. `fullPolicySync` does roughly this:

1. Run `iptables-save` for the filter table into a `bytes.Buffer`, one per IP family.
2. Append the policy chains (`KUBE-NWPLCY-*`) and, for every pod on the node, a firewall chain (`KUBE-POD-FW-*`) with its rules.
3. Strip stale rules from the buffer.
4. Hand the buffer to `iptables-restore`.

A pod event calls `RequestFullSync`, and so does a periodic timer. Requests go through a channel of capacity 1, and syncs run one at a time. The issue behind the PR (#2191) reports CPU profiles where `fullPolicySync` took 26-58% of each node's k3s process CPU (the whole function, not only this check).

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

## Why it cost so much

`bytes.Buffer.String()` ends in one line in the Go standard library:

```go
return string(b.buf[b.off:])
```

That is a copy of the whole buffer. At this point the buffer holds the full `iptables-save` output plus every rule generated so far. Then `strings.Contains` scans the copy. The rule never matched, so every scan ran to the end.

So each pod cost one allocation, one copy and one scan of the entire table. Total work is pods times table size. The table also grows with pods, because each pod adds a chain and its rules. In the benchmark, 80 copies of a 1 MiB buffer is 83,886,080 bytes. The measured 90,489,076 B/op is close to that.

Using `bytes.Contains(buf.Bytes(), ...)` would have removed the copy, since `Bytes()` returns a slice that aliases the buffer. It would not have removed the scan. The cost would still scale with pods times size.

## The fix, in two steps

My first version replaced the buffer search with a per-family set of the rules written in this sync:

```go
logRule := strings.Join(args, " ")
if loggedDrops[ipFamily][logRule] {
	continue
}
loggedDrops[ipFamily][logRule] = true
```

On the benchmark it gave the same result as the final version. Then the maintainer, @aauren, reviewed it. The review comment said they would argue "we should just drop this check altogether", because "as far as I can tell this is dead code and never has the potential of actually coming back with a match, unless you see it differently?"

That argument holds up. The rule text contains:

- the pod's chain name, which is `KUBE-POD-FW-` plus 16 characters of base32(sha256(namespace + podName + version));
- a `<namespace>/<name> log-drop` comment.

The version is derived from the sync's start time in nanoseconds, so rules left by an earlier sync carry a different chain name. For a match inside one sync, the same pod would have to come through the loop twice in the same family. The maintainer's read was that a real cluster cannot do that. I could not find a way either.

My description had already said that rules from an earlier sync could never match, because the chain name contains the sync version and `iptables-save` prints the rule differently (`10/min` instead of `10/minute`, and a different match order). My set was guarding against a case that could not happen. I agreed, deleted the check and the set, and dropped the two tests that only covered them. The final diff in `pod.go` is five deletions and no additions. @aauren approved it and merged it on 2026-10-12.

A side effect: while checking whether pods with colliding chain names could trigger the check, I noticed that the hash joins namespace and name with no separator. Pods `a`/`bc` and `ab`/`c` get the same chain name. That is a separate bug, and the maintainer filed it as #2193, which covers more than pod chains. I left it alone in this PR. The `log-drop` comment still differs for those two pods, so the old check did not fire for them either.

## How I measured

First, a test to pin behaviour. `TestSyncPodFirewallChainsDropRules` asserts the exact rules written per chain and per family, for single-stack, dual-stack and colliding-name pods. The PR says it passes on master and on the branch. A change that deletes a check needs a test that agrees with the old code.

Then the benchmark. It builds 80 pods in one IP family and fills a buffer with synthetic policy-chain lines past 1 MiB, as if `iptables-save` had just run:

```go
for i := 0; existing.Len() < 1<<20; i++ {
	fmt.Fprintf(&existing, "-A KUBE-NWPLCY-%016X -m comment --comment \"ns-%d/policy-%d ingress pods\" "+
		"-m set --match-set KUBE-SRC-%016X src -j MARK --set-xmark 0x10000/0x10000\n", i, i%40, i, i)
}

for b.Loop() {
	buf.Reset()
	buf.Write(existing.Bytes())
	npc.syncPodFirewallChains(nil, "bench")
}
```

The numbers are the median of `-count 5`. Allocations per op went from 2115 to 1954, which is 161 fewer. That fits two fewer per pod, the `String()` copy and one `strings.Join`.

Some limits. The loop also resets and refills the 1 MiB buffer on each pass, so that time is inside both numbers, and 477,097 ns/op is an upper bound for the function, so the 320x ratio is understated. The benchmark covers one function, not a whole sync. I did no cluster testing for this PR. The issue's own standalone benchmark, at 84 pods and a 1.2 MB buffer, measured 180-188 ms per sync for the check, which is the same order as mine.

## What I take from it

- A call that looks like a read can allocate. Inside a loop, ask what it costs per iteration.
- Before making a check faster, ask whether it can ever be true. I built a set. The maintainer asked whether the check was needed at all, which was the better answer.
- Pin behaviour with a test that passes before and after, then change the code.

## How others solve it

kube-proxy in iptables mode has the same shape: build the rules in line buffers, then restore them with `NoFlushTables`. I found no search of those buffers in it. It tracks the chains it wants in a set (`activeNATChains := sets.New[utiliptables.Chain]()`). It reads `iptables-save` only to list chain names with `GetChainsFromTable` and diffs them against that set. In large clusters it does that at most once per sync period, because, as its own comment says, `iptables-save` "can take several seconds to run on hosts with lots of iptables rules". It also has a `doFullSync` flag, so not every sync rebuilds everything.

Keep the state in a data structure, and do not search the output buffer once per item.

If you run kube-router or k3s with many pods per node, I would like to hear what your sync times look like. Reply on [X](https://x.com/spicysharkco).
