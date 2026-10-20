---
layout: post
title: "One unquoted attribute skips the whole tag: PathConverter and B64 in pymdown-extensions"
date: 2026-10-20
project: pymdown-extensions
pr: https://github.com/facelessuser/pymdown-extensions/pull/2998
status: merged
tags: [observability-data, python, testing]
---

Raw HTML such as `<img src="a.png" width=100>` kept its `src` as written in pymdown-extensions, a Python library of Markdown extensions: PathConverter did not rewrite it and B64 did not inline it, because `width=100` has no quotes. [PR #2998](https://github.com/facelessuser/pymdown-extensions/pull/2998) fixes both, and it was merged under a day after I opened it.

## When it happens

```py
import markdown

cfg = {'pymdownx.pathconverter': {'base_path': '/base', 'absolute': True}}
for src in ['<img src="a.png" width=100>', '<img src="a.png" width="100">']:
    print(markdown.markdown(src, extensions=['pymdownx.pathconverter'], extension_configs=cfg))
# before the fix:
# <p><img src="a.png" width=100></p>
# <p><img src="/base/a.png" width="100"></p>
```

I saw the same skip in 6.0.0, 8.0, 10.0 and 12.3.

## How the tag pattern reads attributes

Both extensions run a postprocessor over the rendered HTML. In `pymdownx/pathconverter.py` and `pymdownx/b64.py`, `RE_TAG_HTML` must match a whole opening tag: the name, a run of attributes, then `>` or `/>`. Before the fix an attribute was a name with an optional value in double or single quotes. Only a tag the pattern matches reaches `repl`, which rewrites the `src` or `href` in it. At `width=100` the run stopped at `=`, the match failed, and the tag stayed as it was, with no error.

## Why the gap existed

Both attribute lines are unchanged since the extensions were added, in November 2014. They list two quote styles, and HTML also allows a value with no quotes. No tag in `test_pathconverter.py` or `test_b64.py` had one.

## The fix

The `pymdownx/pathconverter.py` diff as merged, without context lines. The `pymdownx/b64.py` line is the same:

```diff
@@ -50 +50 @@ RE_TAG_HTML = r'''(?xus)
-        (?P<attr>(?:\s+[\w\-:]+(?:\s*=\s*(?:"[^"]*"|'[^']*'))?)*)
+        (?P<attr>(?:\s+[\w\-:]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'`=<>]+))?)*)
```

The new alternative takes a run of characters with no whitespace, quote, backtick, `=`, `<` or `>`. The tag now matches and `repl` sees it: the first line gives `/base/a.png` and keeps `width=100` as written.

## The test

`test_unquoted_attribute` joined `TestRelative`, `TestAbsolute` and `TestAbsoluteFileScheme` in `test_pathconverter.py`, and `TestB64` in `test_b64.py`. Each input puts an unquoted attribute after the `src` or `href` to convert: `width=100` for the images, `class=link` for the links. The expected output converts the `src` or `href` and keeps the unquoted attribute as written. In each of three runs, all four tests failed without the fix and passed with it.

## What the maintainer did

@facelessuser wrote “Seems to be a reasonable change; I'll take a closer look once I have some time.” They merged it hours later. The merged change equals mine. A later commit added the changelog entry.

## The lesson

- Build rows from the language's rules, not from the pattern's alternatives. A pattern that lists two quote styles invites rows with two; HTML has a third form.
- Give a character class the characters it exists to allow. The rows use `100` and `link`: with `\w+` in its place all four still pass, and `<img src="a.png" width=50%>` is skipped again (mutant run).

If a pattern of yours skips a whole tag over one attribute, reply on [X](https://x.com/spicysharkco).
