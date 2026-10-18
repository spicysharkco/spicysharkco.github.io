---
layout: post
title: "Loading model weights can run a stranger's code"
date: 2026-10-17
---
Downloading a model and loading it feel like reading data. With a pickle-based checkpoint
they are not: `torch.load` on one, by default until torch 2.6, unpickles it — and
unpickling runs code the file's author chose, during the load, before you ever call the
model.

## Why a weights file is code
Pickle is not a data format. It is a short instruction stream for rebuilding Python
objects. A class can define `__reduce__`: the pickler calls it when the file is *written*,
to record how to rebuild the object as a callable plus its arguments. The unpickler calls
that callable when the file is *read*. Whoever writes the file picks the callable, so a
crafted checkpoint can name `os.system` and a command, and it fires the moment the file is
deserialized. "Load this `.pt` file" and "run this script" are the same sentence.

## What makes it someone else's bug
On its own that is a local footgun — you ran a file you chose. It becomes a remote bug
when a server loads weights from a location the client names: a model identifier, a bucket
or a file. If the load is the pickle kind and nothing authenticates the caller, an attacker
hands the server a file and the server runs it as itself.

## The fix is boring
Three changes, all unglamorous:

- **Upgrade first, and read `weights_only=True` as a narrowing, not a guarantee.** Torch
  2.6.0 fixed CVE-2025-32434, a remote code execution through
  `torch.load(..., weights_only=True)` in earlier releases. From 2.6 `weights_only=True`
  is also the default, and the unpickler refuses globals outside a short allowlist. That
  closes the known bypass; it does not make an untrusted file safe. The current PyTorch
  serialization notes, which are newer than the 2.6.0 docstrings, say `weights_only=True`
  narrows the remote-code-execution surface, does not guard against denial of service, and
  may still leave room for memory corruption, and the current `torch.load` documentation
  says never to load data from an untrusted source. For files you do not control, prefer
  safetensors.
- **Prefer safetensors.** The file format holds tensors and a small header, and it cannot
  execute code when loaded. That does not make the setup around it safe. A model
  repository can ship its own modeling code, which runs when you load with
  `trust_remote_code=True`, and a file that cannot run code can still carry weights an
  attacker trained to misbehave. Leave `trust_remote_code` off, pin the revision you
  reviewed, and take models from publishers you trust.
- **Authenticate, and choose the locations.** Put authentication in front of any endpoint
  that loads from a caller-supplied location, and decide on purpose which locations it
  will accept.

## What the ecosystem changed
PyTorch 2.6 flipped the default: when you pass neither `weights_only` nor `pickle_module`,
`torch.load` runs with `weights_only=True`. Hugging Face built safetensors as a format that
holds tensors and cannot run code on load. That moves the work to three things: stop
opting out, run a version with the CVE-2025-32434 fix, and use safetensors for anything you
do not control. Every `weights_only=False` still sitting in a server is a standing decision
to run whatever the file says. Two environment variables change the default (checked
against the 2.6.0 source and PyTorch main). `TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD=1` turns it
back off for any call that does not pass `weights_only` itself.
`TORCH_FORCE_WEIGHTS_ONLY_LOAD=1` forces `weights_only=True` even over an explicit `False`,
which makes it a cheap backstop to set on a server, and a call that also passes
`pickle_module` then fails with
`RuntimeError: Can not safely load weights when explicit pickle_module is specified`.
Setting both raises an error.

## A grep checklist
A starting point, not an exhaustive list. Run these over the server and its deployment
files:

- `weights_only=False`: an explicit opt-out.
- `TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD`: the environment opt-out above.
- `pickle_module=`: passing your own pickle module to `torch.load` brings back the full
  unpickler.
- `pickle.load`, `dill.load`, `cloudpickle.load` (and the `loads` forms), `pickle.Unpickler`,
  `joblib.load` and `read_pickle` (`pandas.read_pickle`, `pd.read_pickle`): all unpickle,
  whatever torch version you run. `from pickle import load` is caught at the import line,
  not at the later bare `load(` call.
- `numpy.load(..., allow_pickle=True)`: the default has been `False` since NumPy 1.16.3
  (CVE-2019-6446), and `True` before that; this turns pickle on.
- The torch version you pin, wherever you pin it: `requirements*.txt`, `pyproject.toml`
  (including Poetry), Dockerfiles. Look for `torch[extras]==` forms, a bare `torch` that
  installs whatever is newest, and `pytorch/pytorch:` base images. Then check the version
  that is actually installed.

```sh
grep -rnE 'weights_only *= *False|TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD|pickle_module *=|(pickle|dill|cloudpickle)\.(loads?|Unpickler)\(|from (pickle|dill|cloudpickle) import|joblib\.load\(|read_pickle\(|allow_pickle *= *True' .
grep -rnE '(^|[^[:alnum:]_])torch([^[:alnum:]_]|$)|pytorch/pytorch:' --include='requirements*.txt' --include=pyproject.toml --include='Dockerfile*' .
python3 -c 'import torch; print(torch.__version__)'
```

Misses are expected: aliased imports and loaders built on pickle, `shelve` for one, will
not match.

This is the same lesson as
[the gate post]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %}):
the finding lives in the default. On torch 2.6 or later, with no `pickle_module` and
neither environment variable set, `torch.load(path)` is the restricted load;
`torch.load(path, weights_only=False)` reads like a back door. Check which one your server
calls, which version it runs — and who gets to choose the path.

*Updated 2026-10-18: Corrected the `weights_only=True` advice: it needs torch 2.6 or
later (CVE-2025-32434), and it narrows the attack surface without making untrusted files
safe, so the advice for files you do not control is safetensors. Fixed the safetensors and
`__reduce__` explanations, cited the current PyTorch docs, added the two
`TORCH_FORCE_*` environment variables and a grep checklist, and renamed the link to the
gate post. Removed unsourced claims, including how many servers and models are affected
and that load endpoints are often unauthenticated, and fixed "Two changes" to three.
Every change is in the
[file history](https://github.com/spicysharkco/spicysharkco.github.io/commits/main/_posts/2026-10-17-loading-model-weights-can-run-a-strangers-code.md).*
