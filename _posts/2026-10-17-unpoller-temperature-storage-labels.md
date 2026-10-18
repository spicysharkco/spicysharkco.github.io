---
layout: post
title: "Label names declared in the wrong order: temperature and storage in unpoller"
date: 2026-10-17
project: unpoller
pr: https://github.com/unpoller/unpoller/pull/1113
status: merged
tags: [observability-data, prometheus, labels]
---

`unpoller_device_temperature_celsius` and `unpoller_device_storage` put the tag and the sensor or disk values under the wrong label names, and lost series too. I fixed it in [PR #1113](https://github.com/unpoller/unpoller/pull/1113), merged within half an hour of opening it. It is the mistake [my station-count fix](https://spicysharkco.github.io/2026/10/17/unpoller-station-count-labels.html) removed, in two more descriptors.

## When it happens

With the default of no device tags, a UXG with a `Local` sensor (type `board`, 41), a `CPU` sensor (type `cpu`, 62) and no disk usually exported `{temp_area="", temp_type="CPU", tag="cpu"}` twice, with values 41 and 62. It should export `{temp_area="Local", temp_type="board", tag=""} 41` and `{temp_area="CPU", temp_type="cpu", tag=""} 62`. A scrape rejects the second series.

A USW exported `{temp_area="", temp_type="general", tag="board"} 38` instead of `{temp_area="general", temp_type="board", tag=""}`. USG, UDB, UBB and the UXG disks shifted the same way. UDM was right.

The USW Insights dashboard groups its temperature panel by `name,temp_area,temp_type`. Its legend read `Switch general C` and reads `Switch general board C` with the fix. A query on `temp_type="general"` needs `temp_area="general"`.

## How the labels are built

`descDevice` in `udm.go` listed `tag` last in both descriptors. UXG, USG, USW, UDB and UBB pass labels that already end with the tag, then append the sensor or disk values. UDM passes the tag last.

UXG and USG build the labels with `append(baseLabels, tag)`: five with room for eight. The sensor and disk appends write into the same spare slots, and the exporter reads the labels later. In the test, a UXG's sensors came out with the mount point and name of its last disk.

## Why the gap existed

Commit 6d85ea7 added `tag` to the device descriptors in January 2026. It declared `tag` last and gave most callers labels that end with it. No test read these series back.

## The fix

Declare `tag` first in both descriptors, pass it first in UDM, and clip the labels in UXG and USG. The order of names in a descriptor does not show in the scrape output. Two of the hunks, minus the `slices` import; the storage descriptor, the UDM calls and the USG clip are the same moves:

```diff
@@ -49 +49 @@ func descDevice(ns string) *unifiDevice {
-			append(labels, "temp_area", "temp_type", "tag"), nil),
+			append(labels, "tag", "temp_area", "temp_type"), nil),
@@ -28 +30 @@ func (u *promUnifi) exportUXG(r report, d *unifi.UXG) {
-		labels := append(baseLabels, tag)
+		labels := slices.Clip(append(baseLabels, tag))
```

## The test

`TestExportTemperatureAndStorageKeepTheirLabels` exports UXG, USG, USW, UDB, UBB and UDM payloads, builds each series from its descriptor, and checks the label values of every temperature series and of the UXG and UDM disks.

Before the fix it failed for every device but UDM. After the fix it passes. I also reverted five groups of lines one at a time: the descriptors, the UDM temperature call, the UDM storage calls, the UXG clip and the USG clip. Each revert failed the test.

## What the maintainer did

@platinummonkey merged it with no comment or review. It was my second unpoller PR; the first merged the same day.

## The lesson

- After a fix, look for the same mistake in sibling definitions.
- Reordering a shared definition moves the callers that were right. UDM needed edits too, so it belongs in the test.
- Revert each hunk of a fix alone and run the test. A hunk whose revert passes is a hunk nothing guards.

If a descriptor and its callers sit in your own exporter, reply on [X](https://x.com/spicysharkco).
