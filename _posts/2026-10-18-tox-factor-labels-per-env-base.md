---
layout: post
title: "One label dict for every table: positional factor labels in tox"
date: 2026-10-18
project: tox
pr: https://github.com/tox-dev/tox/pull/4174
status: merged
tags: [observability-data, python, config]
---

In tox, a Python tool that runs test environments, a project with two `env_base` tables using positional factor labels got empty labels for every table but the last. [PR #4174](https://github.com/tox-dev/tox/pull/4174) fixed it and merged about six hours after I opened it. @gaborbernat added three commits to my first tox PR.

## When it happens

```toml
[env_base.sync]
factors = [["oci", "python"], ["pw", "tt"]]
description = "Sync {factor:0} to {factor:1}"

[env_base.build]
factors = [["api", "web"], ["debug", "release"]]
description = "Build {factor:0} {factor:1}"
```

`tox c -e sync-oci-pw,build-api-debug -k description` printed `Sync to ` for the first environment, not `Sync oci to pw`. The second was right: `build` is the last table. An `env_list` `product` with groups hit it too: its labels overwrote the table's.

## How factor labels work

`TomlPyProject` kept the groups of every `env_base` table in one dict, then added the `env_list` groups. `replace_factor` found the group there and returned the value the environment name holds. For `sync-oci-pw`, `{factor:0}` found the group of `build`. The name holds none of its values, so it came back empty.

## Why the gap existed

The one dict came with `{factor:label}` in [#3865](https://github.com/tox-dev/tox/pull/3865). Old label tests used one `env_base` table or none.

## The fix

`_build_env_base_map` keeps a dict per table. An environment from a table gets the `env_list` labels with that table's labels on top. Other environments get the combined dict. `Config.factor_labels` now takes the environment name. The diff is trimmed to one hunk each from `replacer.py` and `toml_pyproject.py`, without context lines or hunk-header names.

```diff
@@ -390 +390 @@
-    group = conf.factor_labels.get(label)
+    group = conf.factor_labels(conf_args.env_name).get(label)
@@ -108,0 +115,5 @@
+    def factor_labels(self, env_name: str | None) -> dict[str, FactorGroup]:
+        if env_name in self._env_base_generated:
+            return {**self._env_list_labels, **self._env_base_labels[self._env_base_generated[env_name]]}
+        return self._factor_labels
+
```

## The test

`test_env_base_positional_factor_per_base` runs `tox c -k description` on `sync-oci-pw` and `build-api-debug`; a second test covers the `env_list` product. At my last commit both failed on the unfixed code in three of three runs, and three test files passed with the fix (113 tests).

## What the maintainer did

@gaborbernat squash-merged it. Their first commit changed what my lookup starts from: mine began from the combined dict, so another table's label still resolved if the name held one of its values. Their new test expects `missing` for `{factor:target:missing}` in `second-x`; my code gave `x`. The second moved `FactorGroup` to `tox.config.types`, documented it, and removed the docs ignore I had added. The third swapped my `Source.factor_labels` default for a `FactorLabelSource` protocol that `Config` checks with `isinstance`.

## The lesson

- Ask for a key that lives on the other side. My tests passed while another table's label still resolved.
- A type in a public signature is public. `Config.factor_labels` returned `FactorGroup` values, from a private module. I added a docs ignore for it; the maintainer made it public.
- An optional hook can be a protocol, not a base-class default. The caller checks for it; other sources need no stub.

If you merge state from several tables into one dict, reply on [X](https://x.com/spicysharkco).
