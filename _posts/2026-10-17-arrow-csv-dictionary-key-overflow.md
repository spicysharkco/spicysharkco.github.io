---
layout: post
title: "I made the arrow-csv reader return an error on dictionary key overflow"
date: 2026-10-17
---

An `arrow-csv` reader panicked when a `Dictionary(Int8, Utf8)` column had 129 distinct values in one batch. I changed it to return `ArrowError::DictionaryKeyOverflowError` in [PR #11628](https://github.com/apache/arrow-rs/pull/11628). It closes [issue #11623](https://github.com/apache/arrow-rs/issues/11623) and was merged on 2026-10-17, about 10 hours and 38 minutes after I opened it. A value that cannot be parsed already came back as an error from the reader. This case now does too.

## When it happens

`ReaderBuilder` in `arrow-csv` takes a schema. A column can be a dictionary, for example `Field::new_dictionary("c", DataType::Int8, DataType::Utf8, false)`: each distinct string is stored once, and each row holds an `Int8` key. The issue notes that default schema inference never produces dictionary types, so you reach this by passing your own schema.

A dictionary key is a non-negative index, so an `Int8` key runs from 0 to 127 and one dictionary holds at most 128 values. The reader decodes rows into batches, 1024 rows by default, and builds a new dictionary for each batch. The limit applies to distinct non-null values in a batch, not in the file. With 128 distinct values in the batch, `next()` returned the batch. With 129, it panicked with `dictionary key overflow: DictionaryKeyOverflowError`. The issue also shows `Int32` keys reading the same 129 values without trouble.

## Why it panicked

In `arrow-csv/src/reader/mod.rs`, `parse()` has one arm per dictionary key type. Each arm mapped the rows to `Option<&str>` and called `.collect::<DictionaryArray<Int8Type>>()`, or the matching key type, inside `Ok(Arc::new(...))`. A `collect()` returns the array itself, so it has no way to return an error.

`FromIterator` for `DictionaryArray` creates a `StringDictionaryBuilder` and calls `extend` on it. `extend` calls `append_option`, which calls `append_value`. That method is the infallible one: `self.append(value).expect("dictionary key overflow")`. The builder also has a fallible `append`, documented as "Returns an error if the new index would overflow the key type." The CSV arms never reached it.

For comparison, a bad value in a primitive column already returned an `ArrowError::ParseError` that names the value, the column and the line. The overflow error has no fields, so it carries none of those. Its message is only "Dictionary key bigger than the key type".

## The fix

Each of the eight arms now calls one helper, `build_dictionary_array::<K>()`. It builds the column with `StringDictionaryBuilder`, sends null values to `append_null` and every other value to `append(s)?`. This is an excerpt: one of the eight arms and the helper's body, with indentation removed. The `@@` lines are my labels, not the diff's own headers:

```diff
@@ in parse() @@
-DataType::Int8 => Ok(Arc::new(
-    rows.iter()
-        .map(|row| {
-            let s = row.get(i);
-            (!null_regex.is_null(s)).then_some(s)
-        })
-        .collect::<DictionaryArray<Int8Type>>(),
-) as ArrayRef),
+DataType::Int8 => build_dictionary_array::<Int8Type>(rows, i, null_regex),
@@ new helper @@
+let mut builder = StringDictionaryBuilder::<K>::with_capacity(rows.len(), 256, 1024);
+for row in rows.iter() {
+    let s = row.get(col_idx);
+    if null_regex.is_null(s) {
+        builder.append_null();
+    } else {
+        builder.append(s)?;
+    }
+}
+Ok(Arc::new(builder.finish()))
```

`parse()` already collects the per-column results into a `Result`, so the error leaves `flush()` and reaches the caller as `Some(Err(...))` from `next()`. The PR touches one file: 54 lines added and 65 removed. Of the removed lines, 64 are the eight repeated arms, and one is an import line that gained `StringDictionaryBuilder`.

## The test

`test_csv_dictionary_key_overflow` in the same file uses one column with `Int8` keys. A closure takes a count, writes that many rows of distinct strings (`v0`, `v1` and so on) and reads the first batch. Reading 128 rows returns a batch of 128 rows. Reading 129 returns an error that matches `DictionaryKeyOverflowError`. The PR body says that without the change the test panics in `GenericByteDictionaryBuilder`; I did not rerun that for this post.

The test uses only `Int8`. The other seven key types share the helper, and `test_csv_with_nullable_dictionary` already reads all eight, but with three rows, so it reaches none of their limits.

## What the maintainer did

@Jefffrey approved with no review text 9 hours and 53 minutes after I opened the PR, and merged it 45 minutes after that. There were no inline review comments, and the PR was one commit.

## The lesson

- A shortcut such as `collect()` can hide the panicking path of a builder that also has a fallible one. Check which method the shortcut calls before you trust it with input from a file.
- Return the error the same way the neighbouring code does. Bad values already came back as `Err` from the reader, so overflow should too.
- Test both sides of the limit. Reading 128 and 129 values shows where the boundary is. A test with only the failing input could not tell an off-by-one from a reader that rejects everything.

If a parser of yours panics on input its signature says it can reject, reply on [X](https://x.com/spicysharkco).
