---
layout: post
title: "I made the arrow-schema FFI export keep a map's keys-sorted flag"
date: 2026-10-18
---

Exporting a `Field` of `DataType::Map(_, true)` through `arrow-schema`'s C Data Interface dropped the keys-sorted flag, so it came back as `Map(_, false)`. I fixed it in [PR #11630](https://github.com/apache/arrow-rs/pull/11630). It closes [issue #11603](https://github.com/apache/arrow-rs/issues/11603) and was merged on 2026-10-18, 28 hours and 57 minutes after I opened it. It follows [my CSV fix](https://spicyshark.co/2026/10/17/arrow-csv-dictionary-key-overflow.html).

## When it happens

`ArrowSchema` in the [C Data Interface](https://arrow.apache.org/docs/format/CDataInterface.html#the-arrowschema-structure) has a `flags` bitfield. One flag, `ARROW_FLAG_MAP_KEYS_SORTED`, means "for map types, whether the keys within each map value are sorted". Export a sorted map as a bare `DataType` and you import a sorted map. Export it with `FFI_ArrowSchema::try_from(&field)` and `Field::try_from(&c_schema)` returns an unsorted one.

Struct, list, union and `Schema` children are exported as fields, so a sorted map inside any of them was imported as unsorted too.

## How the flags are set

In `arrow-schema/src/ffi.rs`, `Flags` has three bits: `DICTIONARY_ORDERED`, `NULLABLE` and `MAP_KEYS_SORTED`. `TryFrom<&DataType>` sets `MAP_KEYS_SORTED` for `DataType::Map(_, true)`. `with_flags` is `self.flags = flags.bits()`. It assigns, so it replaces earlier bits.

## Why the gap existed

`TryFrom<&Field>` builds the schema from the field's data type, which sets the sorted bit. It then calls `with_flags` with a value built from two things only: nullability and dictionary order. The assignment erased the sorted bit.

The sorted-bit setting, the assigning `with_flags` and the two-flag `Field` conversion were all in place when the FFI code moved into `arrow-schema` in February 2023 (commit 3761ac53). The existing `test_map_keys_sorted` exports a bare `DataType`, so it never took the `Field` path.

## The fix

The whole change outside the tests, as merged:

```diff
@@ -889,6 +889,10 @@ impl TryFrom<&Field> for FFI_ArrowSchema {
             flags |= Flags::DICTIONARY_ORDERED;
         }
 
+        if let DataType::Map(_, true) = field.data_type() {
+            flags |= Flags::MAP_KEYS_SORTED;
+        }
+
         let schema = FFI_ArrowSchema::try_from(field.data_type())?
             .with_name(field.name())?
             .with_flags(flags)?;
```

`with_flags` is unchanged. The PR adds 54 lines to one file: 4 here, 50 in tests.

## The test

The PR adds two tests and a `map_type(keys_sorted)` helper. `test_map_keys_sorted_field` takes sorted and unsorted maps, nullable and not. For each of the four it asserts `map_keys_sorted()` and `nullable()` on the exported schema, then that `round_trip_field` returns an equal field. `test_map_keys_sorted_nested` round-trips both kinds of map as children of a struct, a `List`, a dense union and a `Schema`. Both fail on `main` and pass with the fix.

## What the maintainer did

@Jefffrey asked one review question about how the flag is set. My first version set it differently; the question and my answer are in [three review requests](https://spicyshark.co/2026/10/17/three-review-requests.html). They then approved with no review text and merged it.

## The lesson

- A setter that assigns replaces what an earlier call set. Before you pass a `with_*` method a value you built, check whether it assigns or combines. If it assigns, the value must hold every bit. Here `with_flags` stayed as it was and the caller now builds the whole value.
- Test the path callers use. The old test exported a bare `DataType`; the flag was lost through `Field`, which is how struct, list, union and schema children are exported.
- When a fix adds a flag, assert the others too. The field test checks `nullable()` beside `map_keys_sorted()`, so a fix that traded one flag for the other would fail.

If a `with_*` call in your own code can overwrite what an earlier call set, reply on [X](https://x.com/spicysharkco).
