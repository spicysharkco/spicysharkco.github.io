---
layout: post
title: "I made sql_exporter accept collector references that overlap an earlier entry"
date: 2026-10-17
---

The README's own `collectors` example stopped sql_exporter at startup. I fixed the resolver in [PR #1117](https://github.com/burningalchemist/sql_exporter/pull/1117), merged 2026-10-17, 8 minutes and 4 seconds after I opened it. It counted a collector reference as known only if the reference added a new collector. A glob that matched only collectors already added looked like a typo.

## When it happens

A target or job lists the collectors to run. Each entry is a name or a glob such as `pricing_*`. The README shows this list for `target` and for `jobs`:

```yaml
collectors: [pricing_data_freshness, pricing_*]
```

The README defines one collector, `pricing_data_freshness`, so `pricing_*` matches only that collector. With the README's config and collector file, `-config.check` fails:

```
unknown collector "pricing_*" referenced in target
```

In jobs mode the error ends `referenced in job "db_targets"`. With the glob first, `[pricing_*, pricing_data_freshness]`, the error names `pricing_data_freshness` instead, the exact name that came second.

The trigger is a reference whose matches were all selected by earlier references. Once a collector named `pricing_eu` exists, the glob adds something new and the same list loads.

The README example is enough to reproduce it. Copy its config and collector file, run `-config.check`, and the error appears.

## How references are resolved

`resolveCollectorRefs` in `config/util.go` takes the reference list and a map of collectors by name. For each reference it matches every collector name with `filepath.Match`. A match is appended to the result unless an earlier reference already added it, tracked in a `found` map. A reference with no match fails with `unknown collector %q referenced in %s`. The `%s` is `target`, or `job "name"` in jobs mode.

## Why the gap existed

[PR #194](https://github.com/burningalchemist/sql_exporter/pull/194) added globs to `collectors` and described the `found` map as the way to add a collector matched by several patterns once. That makes overlap legal.

[PR #220](https://github.com/burningalchemist/sql_exporter/pull/220), merged 2023-04-16, added unit tests and reshaped the loop. Its flag for "this reference matched" was set inside the same condition that guards the add, `matched && !found[c]`. One condition did two jobs, so the dedup guard rejected an overlap that #194 was written to allow.

#220's tests covered plain names, a glob plus a name, an empty list and an unknown name. None had a reference whose matches were all added already. The check stood for more than three years.

## The fix

Mark any matching reference resolved. Keep the dedup guard around the add. This is the hunk from `config/util.go`:

```diff
@@ -33,10 +33,12 @@ func resolveCollectorRefs(
 			if err != nil {
 				return nil, fmt.Errorf("bad collector %q referenced in %s: %w", cref, ctx, err)
 			}
-			if matched && !found[c] {
-				resolved = append(resolved, c)
-				found[c] = true
+			if matched {
 				crefResolved = true
+				if !found[c] {
+					resolved = append(resolved, c)
+					found[c] = true
+				}
 			}
 		}
 		if !crefResolved {
```

A collector is still added once, and a reference that matches nothing still fails.

## The test

I added `OverlappingGlobbing` to `TestResolveCollectorRefs` in `config/config_test.go`. The collector map holds `a`, `b`, `c` and `aa`. The references are `aa`, then `a?`. The glob `a?` matches only `aa`, which mirrors the README: a name, then a glob that matches only it. The test expects no error and exactly one collector, `aa`.

Before the fix it fails with `unknown collector "a?" referenced in target`. My PR records 10 of 10 runs failing before and 10 of 10 passing after, and the package suite passing with 16 tests and 5 subtests. It also records the README example passing `-config.check` in both target and jobs mode.

## What the maintainer did

@burningalchemist approved with “Hat trick! 🏒😃 Thanks! 🙌” and merged 31 seconds later. It was my third merged sql_exporter PR, after [#1115](https://github.com/burningalchemist/sql_exporter/pull/1115) and [#1116](https://github.com/burningalchemist/sql_exporter/pull/1116).

## The lesson

- Do not let one condition answer two questions. "Did this reference match anything?" and "did it add anything new?" differ, and deduplication makes them differ.
- Test order and overlap, not only each input alone. A test where every entry adds something new cannot see this bug.
- Run your documented example through your own validator. If it fails, anyone can reproduce the bug from your docs alone.

If an example in your own docs fails the check it ships with, reply on [X](https://x.com/spicysharkco).
