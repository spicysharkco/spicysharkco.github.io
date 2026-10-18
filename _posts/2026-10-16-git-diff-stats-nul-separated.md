---
layout: post
title: "Reading git output with -z, and the test that could not run on Windows"
date: 2026-10-16
project: oh-my-openagent
pr: https://github.com/code-yeongyu/oh-my-openagent/pull/10133
status: merged
tags: [observability-data, git, testing]
---

oh-my-openagent prints a `[FILE CHANGES SUMMARY]` block after a task finishes. It lists modified, created and deleted files with their line counts. The block was wrong for renamed files, for non-ASCII names and for some untracked files. I fixed it by reading git's output NUL-separated ([PR #10133](https://github.com/code-yeongyu/oh-my-openagent/pull/10133), merged 2026-10-15 as [6c161a1](https://github.com/code-yeongyu/oh-my-openagent/commit/6c161a1383b0c6fa928fe71ca1ea08962d4cff5a)).

The parsing was not what review changed. The first Windows CI run failed on the test setup I had written, and the committed bundle went stale. @code-yeongyu diagnosed both. This post is about those two.

## What the summary got wrong

`collectGitDiffStats` ran three commands without `-z`: `git diff --numstat`, `git status --porcelain` and `git ls-files --others`. It then joined the three lists by path. Git's line output did not match how the code joined the three lists. The PR description lists the results:

- A renamed file appeared under `old_name.txt => new_name.txt`, or `d/{sub => sub2}/x.txt` when only a directory changed, instead of its new name.
- A path with non-ASCII characters or a tab came out C-quoted, for example `"caf\303\251.ts"`. An untracked one also lost its line count (+0), because the quoted name could not be read from disk.
- An untracked file with a space in its name, or inside a new untracked directory, was listed under "Modified files" instead of "Created files". Status collapses a new directory to a single `?? newdir/` entry, so its status entry did not match the per-file path.

An excerpt of the before and after output from the PR description:

```text
Before (dev):
Modified files:
  d/{sub => sub2}/x.txt  (+0, -0)
  old_name.txt => new_name.txt  (+1, -0)
  newdir/one.txt  (+1, -0)
  newdir/two.txt  (+2, -0)
  with space.ts  (+2, -0)

Created files:
  "caf\303\251.ts"  (+0)

After (this branch):
Modified files:
  d/sub2/x.txt  (+0, -0)
  new_name.txt  (+1, -0)

Created files:
  café.ts  (+3)
  newdir/one.txt  (+1)
  newdir/two.txt  (+2)
  with space.ts  (+2)
```

The PR follows [#5690](https://github.com/code-yeongyu/oh-my-openagent/pull/5690), which mapped `R` status lines to the destination path in the line parser. That fixed the status side. The numstat side still printed `old => new`.

## The fix

All three commands now read NUL-separated, with rename detection pinned on so `diff.renames` and `status.renames` in the repository config cannot change the result:

```text
git diff --numstat -z -M HEAD
git status --porcelain -z --find-renames --untracked-files=all
git ls-files -z --others --exclude-standard
```

`--untracked-files=all` gives every untracked file its own status entry, which fixes the new-directory case. The untracked line counts the collector builds by hand are NUL-terminated too.

A rename is the awkward record. With `-z`, numstat prints `added\tremoved\t\0old\0new\0`: the path field is empty and the two names follow as separate fields. The new parser handles that with one extra branch:

```ts
let path = field.slice(secondTab + 1)
if (path === "") {
  index += 2
  path = fields[index]
}
```

A rename is reported once, under the destination. A binary file shows `-` for both counts, and `toCount` turns that into 0. The status parser does the same job for porcelain records: for a rename or copy it skips the source field, and it maps a copy destination to "added", because `git diff -M` without `-C` reports a copy as a new file.

One consequence I had to handle. `-z` output is never quoted, so a file name that holds a newline would print as extra lines inside the summary. So `formatFileChanges` now prints a path with a control character, U+2028 or U+2029 JSON-quoted with escapes, and leaves every other path unchanged. `café.ts`, `한글.md` and `with space.ts` stay as they are.

## How the test catches it

The test file, `collect-git-diff-stats.real-git.test.ts`, has 12 cases that run against throwaway repositories with real git. They cover untracked names with spaces and non-ASCII characters, a staged rename with edits, a path with a tab, untracked files in a new directory, an intent-to-add rename, a copy with `status.renames=copies`, a binary rename, and `diff.renames` and `status.renames` set to false.

I ran the new tests against the unmodified `dev` sources and against the branch. On `dev`: 2 pass, 18 fail, and all 12 real-git tests fail. On the branch: 20 pass, 0 fail. The 20 are the 12 real-git tests and 8 formatter tests, and the 2 that pass on `dev` are formatter tests that check plain paths stay unchanged.

The setup keeps each test away from the machine's git configuration. I set `GIT_CONFIG_NOSYSTEM`, author and committer names, and I pointed `GIT_CONFIG_GLOBAL` at `os.devNull`:

```ts
import { devNull, tmpdir } from "node:os"

const HERMETIC_GIT_ENV: Record<string, string> = {
  GIT_CONFIG_GLOBAL: devNull,
  GIT_CONFIG_NOSYSTEM: "1",
  // author and committer names ...
}
```

CI reported two failing jobs, and only the Windows test job failed on this setup.

## What review changed

@code-yeongyu approved the workflow runs, and wrote that they "read the diff before approving CI". They called the change "a careful fix" and said the real-git test "makes it believable". Then CI ran, and they reported two failing jobs, "both small and both in this PR".

- **The Windows `git init` failure.** Every case in the real-git test file failed at `git init` on `windows-latest` with `fatal: unable to access '\\.\nul': Invalid argument`. In their words, `GIT_CONFIG_GLOBAL: devNull` "resolves to `\\.\nul` on Windows, and git can't open that as a config file". They proposed "Pointing `GIT_CONFIG_GLOBAL` at an empty file inside the temp dir (written in `beforeEach`)", which "works on every OS and keeps the test hermetic". That was their idea. I implemented it in [4d1980e](https://github.com/code-yeongyu/oh-my-openagent/commit/4d1980e1ec7489c412b06d1a8f23b9cd7895caf2):

  ```ts
  root = mkdtempSync(join(tmpdir(), "omo-diff-stats-"))
  writeFileSync(join(root, "gitconfig"), "")
  process.env.GIT_CONFIG_GLOBAL = join(root, "gitconfig")
  repo = join(root, "repo")
  mkdirSync(repo)
  ```

  `afterEach` now removes `root`, which holds both the config file and the repository.

- **The stale bundle.** The second failing job was `senpi-compatibility (ubuntu-latest)`: `omo-senpi extension build is not current: stale-output` for `packages/omo-senpi/plugin/extensions/omo-task.js`. @code-yeongyu explained that `packages/utils` is bundled into the committed extensions, so the bundle needs rebuilding, and that CI checks it "on Linux with node 24 and bun 1.4.2", so a bundle built on another OS may not match byte for byte. They also noted the branch was about 100 commits behind `dev`, and that merging `dev` before rebuilding "avoids a second round". My change added exports to `packages/utils/src/git-worktree/index.ts`, and the bundle carries a digest of every module the bundler read, so the digest changed. The bundled code did not.

- **An offer I did not need.** They wrote that if I would rather not deal with the Linux bundle step, they could push the merge, the test fix and the rebuilt bundle on top of my branch, "keeping your commits and credit". I did the work myself and reported it in a [comment on the PR](https://github.com/code-yeongyu/oh-my-openagent/pull/10133#issuecomment-6182750618).

I did the steps in the opposite order to the advice. I rebuilt the bundle in the same commit as the Windows fix, before merging `dev`. The merge then conflicted in the bundle and in `CHANGELOG.md`. I resolved it and rebuilt the bundle on Linux in [6b67ea7](https://github.com/code-yeongyu/oh-my-openagent/commit/6b67ea718351dde8744f458dee4ce0c6240ae999).

`dev` kept moving while the PR waited. I merged it into the branch three more times before the PR was merged, in 6b9b5b9, d547fbf and 78ca1f0. Two of the four merges conflicted in `CHANGELOG.md` and three in `omo-task.js`, and the bundle was rebuilt on Linux after three of them. The final diff of `omo-task.js` is one line added and one removed.

The review result @code-yeongyu posted for `e16a360` was "approved, with no blocking findings". It said the `-z` parsing "handles renames (the destination path is kept), binary `-` counts and paths with tabs". It said the real-git test "is hermetic: it uses a per-test empty `GIT_CONFIG_GLOBAL` and `GIT_CONFIG_NOSYSTEM`". And it said that in the rebuilt `omo-task.js` "only the header digest changed; the bundled code is byte-identical". The PR was merged on 2026-10-15, after the last `dev` merge and rebuild.

## What I did not check

`git status --find-renames` needs git 2.18, the release that added `status.renames`. On older git the status call fails, the existing `catch` returns an empty list, and the summary reads "No file changes detected." I wrote that in the PR description and added that I had not tested it on old git.

## The lesson

Two checks to make before the first push.

**Anything the platform supplies is not a constant.** `os.devNull` is a path, and it is a different path on Windows. A test that needs "an empty file" should make one in its own temp directory. This costs three lines, and it removes a platform branch.

**CI is more than the test suite.** I ran the tests I had written and the type check. The `senpi-compatibility` job checks a built file that lives in the repository and depends on files I had touched. Read the workflow list before pushing, find every job that compares a committed artifact with a rebuild, and run its check locally.

For committed build output, merge the target branch first, rebuild last, and make the rebuild a single command, because the target branch moves while the PR waits.

If you have been bitten by `\\.\nul` or a stale bundle, reply on [X](https://x.com/spicysharkco).
