---
layout: post
title: "A second stored copy of the bounds: the interval supports in NumPyro"
date: 2026-10-19
project: NumPyro
pr: https://github.com/pyro-ppl/numpyro/pull/2376
status: merged
tags: [observability-data, python, refactoring]
---

`Uniform`, `LogUniform` and `DiscreteUniform` in NumPyro stored their bounds twice: as `low` and `high`, and again inside a cached support. [PR #2376](https://github.com/pyro-ppl/numpyro/pull/2376) derives the interval supports from `low` and `high`, so the second stored copy goes away. It is a refactor, not a bug users hit. @juanitorduz merged it about thirteen hours after I opened it.

## Where the two copies differ

A gradient step on the leaves shows them. It is not a usual pattern.

```python
import jax
import numpyro.distributions as dist

d = dist.Uniform(1.0, 3.0)
u = jax.tree.map(lambda p, g: p + g, d, jax.grad(lambda d: d.mean)(d))
print(u.low, u.support.lower_bound, u.support(1.2))
```

Before the change this printed `1.5 1.0 True`: `low` moved, the support did not. Now it prints `1.5 1.5 False`.

## What the cached support cost

Each class built an interval constraint from its bounds in `__init__` and kept it as `_support`. It also listed `_support` in `pytree_data_fields`, so `low` and `high` were pytree leaves twice. `Uniform` went from four leaves to two, `LogUniform` from eight to four: it also holds a log-space `base_dist`.

## Why there were two copies

`Uniform` cached its support from June 2020, and `DiscreteUniform` from December 2021. In July 2023 `_support` joined the pytree fields, with the `vmap_over` lines to map it. From then on the constructor, the pytree fields and the registration had to agree.

## The fix

The `Uniform` part of the `continuous.py` diff, without context lines or hunk headers. `LogUniform` and `DiscreteUniform` change the same way. `batch_util.py` loses three `dist_axes._support = ...` lines:

```diff
-    pytree_data_fields = ("low", "high", "_support")
-    _support: constraints.Constraint
+    pytree_data_fields = ("low", "high")
-        self._support = constraints.interval(cast(NumLike, low), cast(NumLike, high))
-        return self._support
+        return constraints.interval(self.low, self.high)
```

## The test

`test_interval_support_follows_updated_params` runs for `Uniform` and `LogUniform`. It repeats the gradient step, checks that the support bounds equal `low` and `high`, and checks that a value just below `low` is outside the support. Without the change both cases fail. With it both pass.

## What the maintainer did

@juanitorduz had suggested this in an earlier review. On this PR they said the core "looks good" and asked for three changes. My test also asserted `LogUniform.log_prob`. That reads `base_dist`, not the support, so it did not test this change. They would not frame the gradient step as a "user-facing bug fix". And they asked me to drop `test_interval_dist_pytree_leaves`, which pinned how the classes flatten. I dropped the assertion and the test, reworded the description, and they approved.

## The lesson

- Before keeping an assertion, check which field it reads. `LogUniform.log_prob` reads `base_dist`, not the support, so it proved nothing about the change.
- Test a refactor by behaviour, not layout. The test that stays updates the leaves and reads the support.

If a class stores a value it can compute from its own parameters, reply on [X](https://x.com/spicysharkco).
