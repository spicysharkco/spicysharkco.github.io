---
layout: post
title: "A Bessel function that overflows before the result: the VonMises log-density in PyMC"
date: 2026-10-19
project: PyMC
pr: https://github.com/pymc-devs/pymc/pull/8503
status: merged
tags: [observability-data, python, numerics]
---

In PyMC, a Python library for probabilistic programming, `VonMises.logp` returned `-inf` for a large concentration `kappa`: from about 92 in float32, from about 710 in float64. I fixed it in [PR #8503](https://github.com/pymc-devs/pymc/pull/8503), merged about two and a half hours after I opened it.

## When it happens

```py
import pymc as pm
import scipy.stats as st

print(pm.logp(pm.VonMises.dist(mu=0.0, kappa=100.0), 0.0).eval())
print(st.vonmises.logpdf(0.0, 100.0))
```

```
without the fix:  -inf                 SciPy: 1.382390243648071
with the fix:     1.382390229428057
```

Under the default cast policy PyTensor makes a Python float that float32 holds exactly, such as `100.0`, a float32 constant, so the eighth digit differs; `100.1` stays float64. A model with `pm.VonMises("x", 0.0, 100.0)` had a log-probability of `-inf` at its starting point.

## How the density is computed

`logp` was `kappa * cos(mu - value) - log(2 pi) - log(i0(kappa))`. `i0`, a Bessel function, grows like `exp(kappa)`. In float32 `i0(91)` is `1.4e38` and `i0(92)` is `inf`; in float64 `i0(709)` is `1.2e306` and `i0(710)` is `inf`. Then `log(inf)` is `inf` and `logp` is `-inf`.

## Why the gap existed

The existing `test_vonmises` takes `kappa` from `Rplus`, whose finite values stop at 100 in float64, far below 710.

## The fix

The `continuous.py` diff, without context lines:

```diff
@@ -3178 +3178 @@ class VonMises(CircularContinuous):
-        res = kappa * pt.cos(mu - value) - pt.log(2 * np.pi) - pt.log(pt.i0(kappa))
+        res = kappa * (pt.cos(mu - value) - 1) - pt.log(2 * np.pi) - pt.log(i0e(kappa))
```

`i0e(x)` is `exp(-x) * i0(x)` for positive `x`, so `log(i0(kappa))` equals `kappa + log(i0e(kappa))`. The `kappa` term joins the cosine term, and for a positive `kappa`, `kappa * (cos(mu - value) - 1)` is never above zero.

## The test

`test_vonmises_large_kappa` compares `logp` at `-0.1`, `0.0` and `0.1` with `scipy.stats.vonmises.logpdf`, `rtol=1e-6`, for `kappa` of `100.0` and `np.float64(1000.0)`. The first is a float32 constant, the second stays float64. Without the fix the `100.0` and `1000.0` cases both fail; with it both pass. Like `test_vonmises`, it is skipped when `floatX` is float32.

## What the maintainer did

@ricardoV94 asked for benchmarks on stability changes, “to understand if it's an absolute improvement or potential trade-off”, with the numba backend as reference. I replied with a table: numba PR/main 0.99 to 1.02, and about 1% slower on the C linker at 100,000 observations. They wrote “Thanks, looks solid”, approved, and merged it with the code, test and commit message unchanged.

## The lesson

- The overflow point depends on the dtype as well as the formula. Find each one by evaluating the function, then give the test a value above it in each dtype. `100.0` is above the float32 limit; `np.float64(100.0)` is below the float64 limit and passes without the fix.
- Expect to be asked what a stability change costs. Time the function the sampler compiles, `logp_dlogp`, on the maintainer's reference backend, and show ratios between the two versions.

If your log-density takes the log of a fast-growing function, reply on [X](https://x.com/spicysharkco).
