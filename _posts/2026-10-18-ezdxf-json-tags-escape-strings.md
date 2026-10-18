---
layout: post
title: "A template that quoted without escaping: JSON tag export in ezdxf"
date: 2026-10-18
project: ezdxf
pr: https://github.com/mozman/ezdxf/pull/1433
status: merged
tags: [observability-data, python, json]
---

`export_json_tags()` in ezdxf, a Python library for DXF drawing files, wrote JSON that `json.loads` rejected, or read back as a different string, when a string value held a backslash or a double quote. I fixed it in [PR #1433](https://github.com/mozman/ezdxf/pull/1433), merged about three and a half hours after I opened it.

## When it happens

Every MTEXT with a `\P` line break does it:

```python
import json

import ezdxf
from ezdxf.document import export_json_tags

doc = ezdxf.new()
doc.modelspace().add_mtext("first\\Psecond")
json.loads(export_json_tags(doc))
```

The text is `first\Psecond`, with one backslash. `json.loads` raised `JSONDecodeError: Invalid \escape`. A double quote in a value broke the output too, in the compact and the verbose format.

## How the export works

`export_json_tags()` runs the document through `JSONTagWriter`, which writes the DXF tags as `[group-code, value]` pairs. Every string value goes into this template. The verbose format sends numbers through it too; the compact format writes numbers and vertices without it:

```python
JSON_STRING = '[{0}, "{1}"],\n'
```

## Why the gap existed

`JSONTagWriter` has put the value between quotes as it is since it was added in March 2024. The next day the compact format renamed the template `JSON_STRING`. The old tests export a new drawing, or binary data that comes out as hex. None has text with a backslash or a double quote.

## The fix

The template no longer adds the quotes. `write_tag2` encodes the value with `json.dumps(str(value), ensure_ascii=False)`. The diff is trimmed: it leaves out the one-line `import json` hunk and shows no context lines.

```diff
@@ -275 +276 @@ class JSONTagWriter(AbstractTagWriter):
-    JSON_STRING = '[{0}, "{1}"],\n'
+    JSON_STRING = "[{0}, {1}],\n"
@@ -315 +316,2 @@ class JSONTagWriter(AbstractTagWriter):
-        self._stream.write(self.JSON_STRING.format(code, value))
+        text = json.dumps(str(value), ensure_ascii=False)
+        self._stream.write(self.JSON_STRING.format(code, text))
```

## The test

`test_export_json_tags_escapes_string_values` runs in both formats. It adds an MTEXT with the text `first\Psecond "quoted"`, exports the document, parses the result with `json.loads`, loads it with `load_json_tags()` and compares the text with the input.

Before the fix both cases failed. After it all eight tests in the file passed.

## What the maintainer did

@mozman merged it without a review comment. It was my first ezdxf PR.

## The lesson

- Let the encoder quote the value. A template with quotes around `{1}` says that no value holds a quote or a backslash.
- Put the format's reserved characters in the test. Parse the output with a real parser and load it back with the library's own loader. A new drawing holds no value that needs escaping.
- Check a library default against the output you must keep. `json.dumps` escapes non-ASCII text unless `ensure_ascii=False`, and the old template wrote it as it was. I compared old and new output for a drawing with non-ASCII text, a line and a polyline. They differed in two lines that change on every export.

If a writer in your own code wraps values in quotes with a template, reply on [X](https://x.com/spicysharkco).
