---
layout: post
title: "Encrypting every record to read one name: a fix in external-dns"
date: 2026-10-16
---

external-dns synchronizes Kubernetes Services and Ingresses with DNS providers. With TXT encryption on, its TXT registry rendered and encrypted the labels of every record it owned, on every `Records()` call, only to read the name of the result. I sent the fix ([PR #6805](https://github.com/kubernetes-sigs/external-dns/pull/6805), merged 2026-10-16). In the PR's benchmark, with encryption on and an in-memory provider, one `Records()` call over 1,000 endpoints went from 512.59 ms to 23.47 ms and allocated 96.06% less. Then @mloiseleur found a case that my version, and the code it replaced, both got wrong, and sent a follow-up commit. That commit is part of the merged PR.

## What was slow

The TXT registry keeps an ownership record for each DNS record it manages: a TXT record whose value holds labels such as the owner ID. `TXTRegistry.Records()` reads the provider's records and attaches those labels to the endpoints. It also has a migration block, marked `TODO: remove this migration logic in some future release`, for TXT records created before the current naming format. For every record this instance owns, the block checks that the current-format ownership TXT exists. If it does not, it sets `txt/force-update` on the endpoint, so the next apply creates it.

To find the missing ones, the old code called `generateTXTRecord` for every owned endpoint. That builds the whole TXT endpoint: it serializes the labels and, with `--txt-encrypt-enabled`, encrypts them with AES-GCM. In `endpoint/crypto.go`, `EncryptText` gzips the text before it seals it. The caller then read `DNSName` from the result and dropped the rest.

`Records()` runs on every reconcile loop, so a deployment with encryption on pays this on every loop, for every owned record. The `--txt-cache-interval` flag makes `Records()` return a cached result inside the interval, but it is off by default. I measured with it off.

## How I measured it

I added `BenchmarkTXTRegistryRecords`. One iteration is one `Records()` call in the steady state of a loop. An in-memory provider holds N endpoints, alternating A and CNAME. Every one is owned by the registry and already has its ownership TXT. I ran 1,000 and 10,000 endpoints, with encryption on and off, and compared `master` with the branch using benchstat:

```
go test ./registry/txt -run '^$' -bench BenchmarkTXTRegistryRecords -benchmem -count=10
```

Time and allocation per `Records()` call, from the PR description (n=10, p=0.000 for every difference):

| Input | Time, before to after | Allocated, before to after |
|---|---|---|
| encryption on, 1,000 endpoints | 512.59 ms to 23.47 ms (-95.42%) | 1137.14 MiB to 44.81 MiB (-96.06%) |
| encryption on, 10,000 endpoints | 3076.0 ms to 244.3 ms (-92.06%) | 11369.5 MiB to 447.3 MiB (-96.07%) |
| encryption off, 1,000 endpoints | 8.356 ms to 5.552 ms (-33.56%) | 3.071 MiB to 2.590 MiB (-15.68%) |
| encryption off, 10,000 endpoints | 90.86 ms to 64.48 ms (-29.03%) | 29.86 MiB to 25.05 MiB (-16.11%) |

The benchmark checks its own fixture before timing. It reads `Records()` once and fails if the count is wrong, a record is not owned by the registry, or any record already carries the force-update flag.

Two limits. The provider is in memory, so there is no API latency. And the ratios are for this one step, not for a whole reconcile loop.

## The fix before the follow-up

Compute the name of the ownership record and skip the value. This is the helper on my branch after the first review round, and the call site:

```go
// txtRecordName returns the name generateTXTRecord gives the ownership TXT record of r, without
// rendering its value. It returns "" when no such record can be generated.
func (im *TXTRegistry) txtRecordName(r *endpoint.Endpoint) string {
	txt := endpoint.NewEndpoint(im.mapper.ToTXTName(r.DNSName, r.RecordType), endpoint.RecordTypeTXT)
	if txt == nil {
		return ""
	}
	return txt.DNSName
}
```

```go
if txtName := im.txtRecordName(ep); txtName != "" && !txtRecordsSet.Has(txtName) {
	ep.WithProviderSpecific(providerSpecificForceUpdate, "true")
}
```

`endpoint.NewEndpoint` trims the trailing dot and returns nil for a label over 63 characters, so the name matches what `generateTXTRecord` gives. I added two tests. One checks that the computed name equals the generated record's name for prefix, suffix, wildcard and record type variants. The other checks that the force-update flag is still set for a missing TXT record with encryption on.

## What review changed

**A return value.** @ivankatliarchuk's review on 2026-10-14 was marked as changes requested and had one inline comment, which began "Just a nit pick." It said: "An empty string is already a reliable sentinel, there is no other logic empty - false, non-empty - true." My helper had returned `(string, bool)`. I pushed the change about an hour later; that is the form above. That reviewer commented `/lgtm` on 2026-10-15.

**A case I missed.** On 2026-10-16, @mloiseleur wrote: "Thanks, the speedup looks good. I opened spicysharkco/external-dns#1 against your branch with a fix for routed records and a few cleanups. Wdyt?" Their pull request, in my fork, had one commit, 5dfe52a. It made two changes.

The first is a bug fix. My check looked names up in `txtRecordsSet`, a set of TXT names. The old code had the same set. Take an A record that exists twice under one name, as two routed records with set identifiers `set-1` and `set-2`. With the default naming, each has a legacy ownership TXT named `foo.example.org`. Only `set-1` also has the current-format TXT, `a-foo.example.org`. The lookup "is there a TXT named `a-foo.example.org`?" answers yes because of `set-1`'s record. So `set-2` is never flagged, and this migration does not create its current-format TXT.

The fix looks the name up in `existingTXTs`, a map the registry already filled in the same loop. It keys on name and set identifier, and the commit deletes my duplicate set. The lookup became:

```go
if txtName := im.txtRecordName(ep); txtName != "" && !im.existingTXTs.has(txtName, ep.SetIdentifier) {
```

The second change makes `generateTXTRecordWithFilter` build its name with `txtRecordName`. Before, both functions had their own `ToTXTName` call, and only my test said they agreed. `txtRecordName` also checks label lengths directly instead of allocating an `Endpoint`. The commit message adds that this "stops Records() from logging over-long names on every call".

The commit also grew the force-update test from two cases to five, including routed records and an alias A record with a legacy `cname-` TXT. I fast-forwarded my branch to 5dfe52a at 21:19 UTC. @mloiseleur approved the PR at 21:22, and the merge bot merged it at 21:30.

My tests could not see this. Both of my force-update cases used one A record with no set identifier, so a lookup keyed on name alone passed them. My PR description says the set of flagged records is "unchanged". That was true of my first version, which kept master's blind spot. It is not true of the merged code: for routed records it flags a record that master did not. The description still says "unchanged".

## Advice for a change like this

- "Same behaviour" is only as good as the old behaviour. When you replace a computation but keep the lookup, check what the lookup is keyed on. A set of names works until two records share a name, so write a fixture where two things share part of an identity.
- Two copies of a derivation plus a test for equality is weaker than one function. The test catches drift in CI after the fact; one function cannot drift.
- Put assertions in a benchmark fixture, so the number stays about the path you named. Then write in the PR what the number covers: one call, one input size, in-memory provider, cache off.
- When a reviewer calls a return value redundant, check what the caller already knows. An empty string was already the answer to "no name".

If a lookup of yours turned out to be keyed on less than it should have been, reply on [X](https://x.com/spicysharkco).
