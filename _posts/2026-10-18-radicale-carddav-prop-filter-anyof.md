---
layout: post
title: "An allof loop where anyof is the default: CardDAV prop-filter in Radicale"
date: 2026-10-18
project: Radicale
pr: https://github.com/Kozea/Radicale/pull/2285
status: merged
tags: [observability-data, carddav, rfc]
---

A CardDAV `addressbook-query` with two conditions in one `prop-filter` returned only the cards that matched both. RFC 6352 says a `prop-filter` is `anyof` unless it says `allof`. I fixed it in [PR #2285](https://github.com/Kozea/Radicale/pull/2285), merged about an hour and twenty minutes after I opened it. It was my first Radicale PR.

## When it happens

A `REPORT` carries a `prop-filter` with no `test` attribute:

```xml
<C:prop-filter name="NICKNAME">
    <C:text-match>test</C:text-match>
    <C:text-match>a</C:text-match>
</C:prop-filter>
```

For a card with `NICKNAME:test`, the server answered 207 with an empty multistatus. RFC 6352 section 10.5.1 makes `anyof` the default, so the card belongs in the reply. An explicit `test="anyof"` was ignored the same way. `test="allof"` was right.

## How the filter is checked

`test_filter` in `report.py` reads the `test` of the outer `filter` and joins its `prop-filter` results with `any()` or `all()`. Inside each, `prop_match` in `filter.py` looped over the children and returned `False` at the first miss.

## Why the gap existed

A loop that stops at the first miss is `allof`. That fits CalDAV, whose `prop-filter` has no `test` attribute (RFC 4791 section 9.7.2). `prop_match` serves both protocols and never read the attribute.

## The fix

For CardDAV, join the child results with `any()` when `test` is `anyof` or missing, and with `all()` otherwise. CalDAV keeps `all()`. The `filter.py` diff is trimmed. It leaves out a one-line blank hunk, and `[...]` stands for the lines inside `match`, where each old `if not check(): return False` became `return check()`.

```diff
@@ -190 +191 @@ def prop_match(vobject_item: vobject.base.Component,
-    for child in filter_:
+    def match(child: ET.Element) -> bool:
@@ -192,11 +193,12 @@ def prop_match(vobject_item: vobject.base.Component,
[...]
-    return True
+
+    matches = (match(child) for child in filter_)
+    if ns == "CR" and filter_.get("test", "anyof") == "anyof":
+        # rfc6352-10.5.1
+        return any(matches)
+    return all(matches)
```

## The test

`test_addressbook_prop_filter_anyof_allof` in `test_base.py` stores a card with `NICKNAME:test` and sends four reports. The filter above, with a `collation` added to each `text-match`, must return the card with no `test` and with `test="anyof"`. With `test="allof"` it must not. A `text-match` that fails beside a `param-filter` that matches must return it.

Before the fix the new test failed on its first request, which came back empty; the five older prop-filter tests passed. After it all six passed in three runs. 21 filter tests passed with it.

## What the maintainer did

@pbiering approved with “Thank you!” and merged it.

## The lesson

- A loop that returns at the first miss has already chosen `allof`. When a spec offers `anyof` and `allof`, find which one the code implements and what it does when the attribute is missing.
- When one function serves two protocols, a rule from one may not hold in the other. CalDAV has no `test`, so the fix checks the namespace and leaves CalDAV alone.
- An attribute the spec defines on two elements needs a reader for each. Radicale read `test` on the outer `filter`, and nothing read it on `prop-filter`. Search the code for every element that carries the attribute.

If a filter in your own server has a default the spec names, reply on [X](https://x.com/spicysharkco).
