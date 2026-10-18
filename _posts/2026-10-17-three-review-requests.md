---
layout: post
title: "Three review requests from this week, and what I changed"
date: 2026-10-17
tags: [field-notes, code-review, open-source]
---

These are three requests from this week. All three arrived on 2026-10-17: two from reviewers and one from a bot's label. Each made me change something small, and each left a rule I now apply. [My earlier post on code review](/2026/10/16/six-things-code-review-caught.html) listed six things review caught. These are three new requests, though the first is a close cousin of that post's second point. Quotes are copied from the threads, and each status line is as of 2026-10-17.

## 1. Set a new flag the way its sibling is set

Status: approved, not merged.

[arrow-rs #11630](https://github.com/apache/arrow-rs/pull/11630) fixes a lost flag. As the PR body puts it: "Exporting a sorted map as a `Field` drops the keys-sorted flag." My first version let `FFI_ArrowSchema::try_from(field.data_type())` set the flag, then merged those flags back into the field's flags. @Jefffrey [asked](https://github.com/apache/arrow-rs/pull/11630#discussion_r4301826890): "is there a reason we dont just directly check against map type here, and rely instead on `FFI_ArrowSchema::try_from` to do it for us and retrieve it from the flags?"

There was no reason. I [replied](https://github.com/apache/arrow-rs/pull/11630#discussion_r4301864717): "No reason. I changed it to check `DataType::Map(_, true)` directly, the same way `DICTIONARY_ORDERED` is set just above." The code now reads:

```rust
if field.dict_is_ordered() == Some(true) {
    flags |= Flags::DICTIONARY_ORDERED;
}

if let DataType::Map(_, true) = field.data_type() {
    flags |= Flags::MAP_KEYS_SORTED;
}
```

@Jefffrey approved after [that commit](https://github.com/apache/arrow-rs/commit/c7da079bf061b00efe966776b923590c93b71652).

The rule: when you add a flag beside existing ones, set it the way they are set, from the data you already have. Do not read it back from the result of another call. A different route gives the reviewer a question to ask.

## 2. Keep the test to the bug, and give a reproduction

Status: open, with the issue also open.

[k3s #14837](https://github.com/k3s-io/k3s/pull/14837) removes the temporary copy of the etcd database that `NewTemporaryStore` leaves behind when the copy or the open fails. Besides a check that the copy is gone after each expected error, my first version added a test case built around a socket file. @brandond asked: "why are you using a socket anyway? does that bear any relevance to the issue you're fixing?" It did not. I removed the case. The test change in the PR is now six added lines, the check after each expected error.

@brandond also asked for an issue: "Please create an issue for this." It should "just describe what you're seeing and how to reproduce it." I filed [#14838](https://github.com/k3s-io/k3s/issues/14838). Its reproduction needs one k3s server with embedded etcd. Start it, wait until it has saved its bootstrap data, stop it, zero the database's two meta pages, and start it again. In my run k3s exited with an error ending in `failed to open database`, and `server/db/etcd-tmp/member/snap/db` was still on disk, the same size as the original database (17M).

The rule: every test case should trace back to the bug, and a case you cannot explain in a sentence comes out. Give a maintainer a reproduction on the real program as well as a unit test. The unit test shows that my fix passes my test. The reproduction shows that someone running k3s can hit the bug.

## 3. Closing keywords go in the PR body

Status: open.

[external-dns #6818](https://github.com/kubernetes-sigs/external-dns/pull/6818) applies the `rfc2136` provider's minimum TTL before reconciliation. Three seconds after I opened it, a bot added the label `do-not-merge/invalid-commit-message`. That was the whole request, and no person had commented. My commit message carried a closing keyword, which the check does not accept in a commit.

I reworded the commit so it has none and pushed it. The label was removed three seconds after the push, about 16 minutes after it appeared. "Fixes #6723." stays in the PR body, and GitHub still links the PR to that issue from there.

The rule: in a repo that checks commit messages, put `Fixes #N` in the PR body and not in a commit. The commit message says what changed, and the body says what it closes. Look at a new PR's labels once after opening. The bot answers within seconds.

If a review request changed how you open a PR, reply on [X](https://x.com/spicysharkco).
