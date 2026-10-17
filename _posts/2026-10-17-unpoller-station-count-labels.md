---
layout: post
title: "I made unpoller put its station counts under the right labels"
date: 2026-10-17
---

`unpoller_device_stations` put the device tag in `station_type` and the station type in `tag`, and lost series too. I fixed it in [PR #1110](https://github.com/unpoller/unpoller/pull/1110), merged within two hours of opening it.

## When it happens

With the default of no device tags, a UAP with 3 user and 2 guest stations exported one series (other labels left out):

```
unpoller_device_stations{station_type="", tag="guest"} 3
```

It should export `{station_type="user", tag=""} 3` and `{station_type="guest", tag=""} 2`. The Users and Guests panels of the UAP Insights and USG Insights dashboards query by `station_type`, so they show no data.

## How the labels are built

`exportSTAcount` in `pkg/promunifi/udm.go` sends each count as `append(labels, "user")` and so on. The callers pass labels that already end with the tag, but the descriptor listed the names as `station_type`, then `tag`. UBB sends its link and `p2p_*` readings through the same metric, with the same swap.

The labels come from `append(baseLabels, tag)`: five with room for eight. Every append in one send writes into the same spare slot. The exporter goroutine reads the labels later, when every series of the device holds the last station type. A scrape rejects series that share all label values and keeps the first. At the default buffer, a UDM with five station types gave one series before the fix and five after.

## Why the gap existed

Commit 6d85ea7 added the `tag` label to all device metric descriptors in January 2026. It put `"tag"` after `"station_type"` in the descriptor and gave the callers labels that end with the tag. No test read a stations series back.

## The fix

Declare `tag` first, and clip the labels so each append gets its own array. The order of names in a descriptor does not show in the scrape output. Hunks from `udm.go` and `ubb.go`, minus the `slices` import each gains:

```diff
@@ -61 +63 @@ func descDevice(ns string) *unifiDevice {
-		Counter:                  prometheus.NewDesc(ns+"stations", "Number of Stations", append(labels, "station_type", "tag"), nil),
+		Counter:                  prometheus.NewDesc(ns+"stations", "Number of Stations", append(labels, "tag", "station_type"), nil),
@@ -164,0 +167,3 @@ func (u *promUnifi) exportSTAcount(r report, labels []string, stas ...unifi.Flex
+	// The labels may have spare capacity. Clip it, or every append below writes into the same array.
+	labels = slices.Clip(labels)
+
@@ -21 +23 @@ func (u *promUnifi) exportUBB(r report, d *unifi.UBB) {
-		labels := append(baseLabels, tag)
+		labels := slices.Clip(append(baseLabels, tag))
```

## The test

`TestExportStationCountsKeepTheirLabels` calls `exportSTAcount` with labels built as the exporters build them, five with room for eight. It checks that each of the five station types keeps its own `station_type`, and the tag stays in `tag`. `TestExportUBBLinkQualityKeepsItsLabels` does the same for a UBB payload.

Before the fix both failed. The first got `map[string]string{"lobby":"handheld"}`: one entry, the tag as the station type and the last station type as the tag. After the fix both pass, and so does the `promunifi` package.

## What the maintainer did

@platinummonkey merged it with no comment. It was my first unpoller PR.

## The lesson

- A label name and its value meet by position. Nothing fails when they disagree, until a dashboard query returns nothing. Read labels back by name in a test.
- `append` onto a slice with spare capacity writes into shared memory. Clip it with `slices.Clip` before you append several times.
- Build test input the way production does. A slice with no spare room would have hidden the second bug.

If a label list or a reused slice sits in your own exporter, reply on [X](https://x.com/spicysharkco).
