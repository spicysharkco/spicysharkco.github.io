---
layout: post
title: "A hold count raised before the path was resolved: acquire in filelock"
date: 2026-10-18
project: filelock
pr: https://github.com/tox-dev/filelock/pull/856
status: merged
tags: [observability-data, python, concurrency]
---

`FileLock.acquire()` in filelock, a Python library for file locks, raised `FileNotFoundError` when the working directory was gone. That was right, but it left the hold count one too high, so a later `with lock:` never let go. I fixed it in [PR #856](https://github.com/tox-dev/filelock/pull/856), my first in filelock, merged about nine hours after I opened it.

## When it happens

A relative-path lock, and a working directory removed before `acquire()`:

```python
import os
import tempfile

from filelock import FileLock

work = os.path.join(tempfile.mkdtemp(), "work")
os.mkdir(work)
os.chdir(work)
lock = FileLock("resource.lock")
os.rmdir(work)

try:
    lock.acquire()
except FileNotFoundError:
    pass
print(lock.lock_counter, lock.is_locked)
```

The last line printed `1 False`. In a new directory, `with lock:` showed `2 True` inside the block and `1 True` after it. A nested `acquire()` leaked a count too once the directory and lock file were gone.

## How acquire counts

`acquire()` adds one to `lock_counter`, and only the outermost `release()` unlocks. An `except` undoes the add if locking fails. After the add, and before the `try`, `acquire()` calls `_canonical`, which resolves the path's parent, the working directory for a relative path. That raises when the directory is gone.

## Why the gap existed

The count has come first since 2015, when it was added. Until February 2026 nothing that could fail stood between it and the `try`. [#481](https://github.com/tox-dev/filelock/pull/481) put `_canonical` there for a deadlock check, which lowers the count by hand before it raises. `_canonical` itself had no such cover.

## The fix

Resolve the path first. The diff is trimmed: no context lines, nothing after each `@@`.

```diff
@@ -1220 +1220 @@
-            # Bump the counter up front; _undo_acquire rolls it back if acquisition fails.
+            canonical = _canonical(self.lock_file)
@@ -1223 +1222,0 @@
-            canonical = _canonical(self.lock_file)
```

## The test

`test_acquire_without_working_directory_leaves_no_hold` runs for four lock types. It removes the working directory, expects the error, enters a new directory, runs `with lock:` and checks the count and `is_locked`. As I opened it, it failed before the fix in all four with `assert (1, True) == (0, False)` and passed after.

## What the maintainer did

@gaborbernat added two commits, approved it and squash-merged it. One deleted both code comments (mine and the old "Bump the counter up front"), gave the test `Final` locals, and put an assert in the `with` block where mine had `pass`. It also added `test_nested_acquire_without_working_directory_preserves_hold`, the nested case my description named and my test skipped. The other, a soft-RW heartbeat test, is unrelated.

## The lesson

- A rollback covers only what sits inside its `try`. Check where a new call that can fail lands: here `_canonical` ran after the count rose and before the `try`.
- Assert inside the `with` block as well as after it. The maintainer's version checks the count, `is_locked` and the lock file there; my `pass` checked nothing.
- Cover the nested path whenever a counter moves. A leak there is a lock that outlives its `release()`.

If your own code raises a counter before a call that can fail, reply on [X](https://x.com/spicysharkco).
