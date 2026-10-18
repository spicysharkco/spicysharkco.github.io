---
layout: post
title: "I made unpoller export a USG's mobile and handheld station counts"
date: 2026-10-18
---

A USG exported its user count as `mobile` and its guest count as `handheld` in `unpoller_device_stations`. I fixed it in [PR #1114](https://github.com/unpoller/unpoller/pull/1114), merged within two hours of opening it. It follows [my station-count fix](https://spicyshark.co/2026/10/17/unpoller-station-count-labels.html) and [my temperature and storage fix](https://spicyshark.co/2026/10/17/unpoller-temperature-storage-labels.html) in the same exporter.

## When it happens

Every adopted USG did this. The `ugw.json` example in the unifi module has 41 user, 0 guest, 7 desktop, 2 mobile and 8 handheld stations. The USG exported (other labels left out):

```
unpoller_device_stations{station_type="mobile"} 41
unpoller_device_stations{station_type="handheld"} 0
```

It should export 2 and 8. The other three series were right. The Mobile and Handheld panels of the USG Insights dashboard query these two series, so they showed the user and guest counts.

## How the counts are passed

`exportSTAcount` takes five counts in the order user, guest, desktop, mobile, handheld. They are all the same type, so any five values compile. `exportUSG` passed `d.UserNumSta` and `d.GuestNumSta` again for the last two. The USG struct has `NumMobile` and `NumHandheld`, and UXG and UDM pass them.

## Why the gap existed

Commit 4d48576 (December 2019, first released in v1.6.2) moved five inline entries into `exportSTAcount`. Before it, the USG exported `d.NumMobile` and `d.NumHandheld`. No test checked a `unpoller_device_stations` series until my station-count fix added one. It checks labels, not values, and calls `exportSTAcount` directly, so it never saw what `exportUSG` passes.

## The fix

Pass `d.NumMobile` and `d.NumHandheld`. The change is one line in `pkg/promunifi/usg.go`, shown without its context lines:

```diff
@@ -112 +112 @@ func (u *promUnifi) exportUSG(r report, d *unifi.USG) {
-		u.exportSTAcount(r, labels, d.UserNumSta, d.GuestNumSta, d.NumDesktop, d.UserNumSta, d.GuestNumSta)
+		u.exportSTAcount(r, labels, d.UserNumSta, d.GuestNumSta, d.NumDesktop, d.NumMobile, d.NumHandheld)
```

## The test

`TestExportUSGStationCountsUseTheirOwnFields` exports a USG with 41 user, 1 guest, 7 desktop, 2 mobile and 8 handheld stations and checks the value of each `station_type`.

Before the fix it failed: `mobile` was 41 and `handheld` was 1, where it expected 2 and 8. After the fix it passes, and so does the package. I also reverted each of the two arguments alone. Each revert failed the test.

## What the maintainer did

@platinummonkey approved and merged it. It was my third unpoller PR.

## The lesson

- A helper that takes several values of one type compiles with any order and any repeat. Check each call against the parameter order, or pass the values as named fields.
- When you move inline code into a helper, compare the new arguments with the old lines. Here the old lines were right.
- Give every input in a test a different value. A fixture with equal counts cannot tell which field was read.

If a helper with many same-type arguments sits in your own exporter, reply on [X](https://x.com/spicysharkco).
