---
layout: post
title: "One local variable, one reference cycle per error reply in redis-py"
date: 2026-10-15
project: redis-py
pr: https://github.com/redis/redis-py/pull/4434
status: merged
tags: [observability-data, python, memory]
---

On 2026-10-15 redis-py merged a small fix I sent: [#4434](https://github.com/redis/redis-py/pull/4434), "Break the ResponseError reference cycle in asyncio read_response". The fix adds four lines and removes one in `redis/asyncio/connection.py`. The rest is the test. The review from [@petyaslavova](https://github.com/petyaslavova) was short: it matches the sync `Connection.read_response` fix from [#2932](https://github.com/redis/redis-py/pull/2932), and the test covers it.

The bug is one Python habit, not one Redis quirk. You may have it in your own code.

## The symptom

Turn the cyclic garbage collector off. Send 1,000 error replies through the asyncio client. Count how many `ResponseError` objects are still alive, using weakrefs. Before the fix: 1000 of 1000. After: 0 of 1000.

I ran this against a live server with two replies: NOSCRIPT from `EVALSHA`, and WRONGTYPE from `LPUSH` on a string. Both gave 1000 / 1000 before and 0 / 1000 after. The sync client already gave 0 / 1000.

The objects are freed once the collector runs. The question is how long that takes, and what else they hold in the meantime. The PR lists replies that hit this: NOSCRIPT from `EVALSHA` (which `AsyncScript` retries on), WRONGTYPE, BUSYGROUP, and MOVED/ASK in cluster mode.

## One local is the whole bug

This is the code that was there:

```python
if isinstance(response, ResponseError):
    raise response from None
return response
```

`response` is a local variable. When `raise` runs, the exception gets a `__traceback__`, and the traceback points at every frame between where it was raised and where it is caught. One of them is the `read_response` frame. That frame owns its locals, and one local is `response`, the exception itself.

So: exception, `__traceback__`, frame, locals, exception. Reference counts on a loop never reach zero. Only the cyclic collector can free it, and until it does, every frame on the traceback stays alive with its locals.

## Why `except ... as e` does not do this

You write `except Boom as e:` all the time and do not leak. Python deletes `e` for you. From the language reference:

> When an exception has been assigned using `as target`, it is cleared at the end of the `except` clause. This is as if:
>
> ```python
> except E as N:
>     foo
> ```
>
> was translated to:
>
> ```python
> except E as N:
>     try:
>         foo
>     finally:
>         del N
> ```

The docs give the reason: "Exceptions are cleared because with the traceback attached to them, they form a reference cycle with the stack frame, keeping all locals in that frame alive until the next garbage collection occurs." [PEP 3110](https://peps.python.org/pep-3110/) added that cleanup in Python 3.0.

The cleanup covers one name. Any other name bound to the exception is yours to clear: a local you `raise`, like `response`, or a copy such as `saved = e`.

## Why it matters

The collector frees these cycles eventually. But the [gc docs](https://docs.python.org/3/library/gc.html) say: "you can disable the collector if you are sure your program does not create reference cycles." Their recipe for a process that calls `fork()` without `exec()` is `gc.disable()` early in the parent, `gc.freeze()` right before `fork()`, `gc.enable()` early in the children. Instagram Engineering has written about dismissing the collector entirely: [Dismissing Python Garbage Collection at Instagram](https://instagram-engineering.com/dismissing-python-garbage-collection-at-instagram-4dca40b29172).

An app that made that bet does not expect a library to make one cycle per error reply. Raise the thresholds, or turn the collector off, and each cycle is held longer, or for good.

## The fix

```python
if isinstance(response, ResponseError):
    try:
        raise response from None
    finally:
        del response  # avoid creating ref cycles
return response
```

You cannot `del response` before the `raise`, because the raise needs it. The `finally` block runs while the exception is already in flight, so the delete comes after the raise has picked the object up. The frame no longer points at the exception. The exception still points at the frame, but a one-way link is not a cycle. When the caller drops the exception, the count hits zero and everything is freed on the spot. `from None` stays as it was: it only hides the implicit `__context__` in the traceback display and plays no part in the cycle.

The sync client has had the same pattern since #2932 (merged 2023-09-14), with the same comment.

## How the test proves it

`test_read_response_error_reply_breaks_reference_cycle` needs no server. It swaps in a parser stub that returns a `ResponseError`:

```python
    error_refs = []

    async def error_reply(*args, **kwargs):
        error = ResponseError("NOSCRIPT No matching script. Please use EVAL.")
        error_refs.append(weakref.ref(error))
        return error

    conn._read_response_from_parser = error_reply

    gc.collect()
    gc_was_enabled = gc.isenabled()
    gc.disable()
    try:
        with pytest.raises(ResponseError):
            await conn.read_response()

        (ref,) = error_refs
        assert ref() is None
```

(Excerpt. `conn = Connection()` comes first, and a `finally` re-enables the collector if it was on.)

Three details make it honest. The error is built inside the stub, so the weakref is the only handle the test has. `pytest.raises` has no `as excinfo`, so the test does not hold the exception either. I checked: bind `excinfo` and the weakref stays alive. And the collector is off, so only reference counting can free it. If a cycle exists, `ref()` is not `None`. The PR says the test fails without the change and passes with it on Python 3.10, 3.11 and 3.14. It is skipped on PyPy, with the reason "Immediate reclamation is a refcounting property".

I checked it a second way. I ran the same stub, 1,000 calls, against redis-py 8.1.0 from PyPI: 1000 / 1000 alive. With the four-line change applied to a copy: 0 / 1000.

## Spot it in your own code

Run a suspect function many times with the collector off, and ask what only a GC pass could free. `gc.DEBUG_SAVEALL` keeps collected objects in `gc.garbage`, so you can read their types:

```python
import gc


class Boom(Exception):
    pass


def leaky():
    response = Boom("nope")
    raise response


def fixed():
    response = Boom("nope")
    try:
        raise response
    finally:
        del response


def with_as():
    try:
        raise Boom("nope")
    except Boom as e:
        pass


def saved_as():
    try:
        raise Boom("nope")
    except Boom as e:
        saved = e


def left_for_gc(fn, calls=1000):
    gc.collect()
    gc.disable()
    gc.set_debug(gc.DEBUG_SAVEALL)  # collected objects go to gc.garbage
    try:
        for _ in range(calls):
            try:
                fn()
            except Boom:
                pass
        gc.collect()
        errors = [o for o in gc.garbage if isinstance(o, Boom)]
        kinds = sorted({type(o).__name__ for o in gc.garbage})
        return len(errors), kinds
    finally:
        gc.set_debug(0)
        gc.garbage.clear()
        gc.enable()


for fn in (leaky, fixed, with_as, saved_as):
    n, kinds = left_for_gc(fn)
    print(f"{fn.__name__:8} {n:4} exceptions left for the GC  {kinds}")
```

On Python 3.14.7 it prints:

```
leaky    1000 exceptions left for the GC  ['Boom', 'frame', 'traceback']
fixed       0 exceptions left for the GC  []
with_as     0 exceptions left for the GC  []
saved_as 1000 exceptions left for the GC  ['Boom', 'frame', 'traceback']
```

On 3.14 the three type names are the cycle: the exception, its traceback and the frame. Python 3.13 and older also list a `tuple`, the exception's `args`. `with_as` is clean because Python deletes `e`. `saved_as` leaks because `saved` is a second name.

## How others solve it

The language solves it for `except ... as` by deleting the name. The standard library solves the other cases by hand. This is `Future.__get_result` in `concurrent/futures/_base.py` from Python 3.14.7:

```python
        if self._exception is not None:
            try:
                raise self._exception
            finally:
                # Break a reference cycle with the exception in self._exception
                self = None
```

Same shape: raise inside `try`, clear the name in `finally`. There the loop runs through `self`, so it clears `self` instead of the exception.

A related PR, [#4439](https://github.com/redis/redis-py/pull/4439), was still open on 2026-10-15. It targets the same kind of cycle in `NodesManager.initialize`, where `exception = e` stays bound after a later startup node answers.

If you have chased a reference cycle like this one, tell me how you found it. Reply on [X](https://x.com/spicysharkco).
