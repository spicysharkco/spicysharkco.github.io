---
layout: post
title: "A support lost in a pytree round trip: Levy in NumPyro"
date: 2026-10-19
project: NumPyro
pr: https://github.com/pyro-ppl/numpyro/pull/2363
status: merged
tags: [observability-data, python, caching]
---

NumPyro, a probabilistic programming library on JAX, lets a distribution cross a `jit` boundary as a pytree. The `Levy` distribution came out the other side with no support. I fixed it in [PR #2363](https://github.com/pyro-ppl/numpyro/pull/2363). @juanitorduz squash-merged it about 38 hours after I opened it.

## When it happens

```python
import jax
import numpyro.distributions as dist

d = jax.jit(lambda: dist.Levy(0.0, 1.0, validate_args=True))()
print(d.support)
print(d.log_prob(2.0))
```

Before the fix, the first line prints `None` and the second raises `AssertionError`. Now they print `GreaterThan(lower_bound=0.0)` and `-2.2086592`, which agrees with SciPy's `-2.2086593`. The maintainer also ran a NUTS model with a `Levy` site inside `contrib.control_flow.scan`. Without the fix it failed with `AttributeError: 'NoneType' object has no attribute 'is_discrete'`. With it, the model ran.

## How a distribution crosses a jit boundary

`tree_flatten` collects the fields named in `pytree_data_fields`. A class that lists none gets the names in `arg_constraints`, here `loc` and `scale`. `tree_unflatten` builds the object with `cls.__new__`, sets those fields and the shape fields, and never runs the class's own `__init__`. Any other attribute is gone, and the class default takes over. For `_support` that default is `None`.

## Why the gap existed

`Levy` was added in January 2025. Its `__init__` built the support and stored it as `_support`, and the class listed no pytree fields. The round-trip test, `test_dist_pytree`, compared each argument, then a sample and its `log_prob`. It never read `support`.

## The fix

The `continuous.py` diff, without context lines or hunk headers:

```diff
-    _support: constraints.Constraint
-        self._support = constraints.greater_than(cast(NumLike, loc))
-    @constraints.dependent_property(is_discrete=False)
+    @constraints.dependent_property(is_discrete=False, event_dim=0)
-        return self._support
+        return constraints.greater_than(self.loc)
```

The support is built from `loc` on every access, so nothing has to survive the round trip.

## The test

I added one line to `test_dist_pytree`: `assert jnp.all(expected_dist.support.eq(actual_dist.support))`. I added `test_levy_lax_map_batched_scale_support`. It builds a `Levy` with a scalar `loc` and three scales inside `lax.map`, then checks that `support` accepts a (2, 3) value and `log_prob` is finite. With `continuous.py` back at the base and the tests kept, the three `Levy` cases of `test_dist_pytree` and the new test fail. With the fix, all four pass.

## What the maintainer did

My first version listed `_support` as a pytree field and added a `vmap_over` registration. @juanitorduz requested changes. The registration, they wrote, "exists only to keep `_support.lower_bound` in sync with `loc`". They suggested I "derive the support on access". I pushed that version. About seven minutes after my reply they approved: "the shape I asked for".

## The lesson

- If a cached value needs a pytree field and a `vmap_over` registration to stay right, stop caching it. Compute it from the parameters on access. The merged fix is one property, and `batch_util.py` stays as it was.
- Put the check in the shared round-trip test. One `.eq` line runs for every distribution that test covers, and it failed for all three `Levy` cases.

If you cache a value in `__init__` on a class that JAX flattens, reply on [X](https://x.com/spicysharkco).
