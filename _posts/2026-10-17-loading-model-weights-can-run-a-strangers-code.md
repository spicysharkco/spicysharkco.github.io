---
layout: post
title: "Loading model weights can run a stranger's code"
date: 2026-10-17
---
Downloading a model and loading it feel like reading data. For a lot of Python servers
they are not. `torch.load` on a checkpoint, by default for years, unpickles it — and
unpickling runs code the file's author chose, during the load, before you ever call the
model.

## Why a weights file is code
Pickle is not a data format. It is a short instruction stream for rebuilding Python
objects, and an object is allowed to define `__reduce__`, which the unpickler calls. A
crafted checkpoint can carry `os.system(...)` that fires the moment it is deserialized.
"Load this `.pt` file" and "run this script" are the same sentence.

## What makes it someone else's bug
On its own that is a local footgun — you ran a file you chose. It becomes a remote bug
when a server loads weights from a place a *client* can influence: a path in the request,
a URL, a bucket name, a LoRA adapter to hot-swap. Add the detail that these load endpoints
are often unauthenticated, and the sequence is complete: an attacker hands the server a
file, and the server runs it as itself.

## The fix is boring
Two changes, both unglamorous:

- Pass `weights_only=True` to `torch.load`. It refuses to call arbitrary globals during
  unpickling and restores tensors only.
- Better, distribute and load weights as **safetensors** — a format that holds tensors and
  nothing executable, so there is no code path to abuse.
- Put authentication in front of any endpoint that loads from a caller-supplied location,
  and decide on purpose which locations it will accept.

## How the ecosystem solved it
PyTorch 2.6 flipped the default: `torch.load` now runs with `weights_only=True` unless you
opt out. Hugging Face built safetensors for exactly this reason, and most of the hub serves
it. So the work left is mostly to stop opting out. Every `weights_only=False` still sitting
in a server is a standing decision to run whatever the file says.

This is the same lesson as [the last post]({% post_url 2026-10-17-the-cheap-gate-i-run-before-reading-an-inference-servers-code %}): the finding lives in the default. `torch.load(path)` reads safe
today; `torch.load(path, weights_only=False)` reads like a back door. Check which one your
server calls — and who gets to choose the path.
