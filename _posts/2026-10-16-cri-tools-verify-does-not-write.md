---
layout: post
title: "A verify target that rewrote the tree: four review points on a cri-tools script change"
date: 2026-10-16
---

`make verify-docs` and `make verify-go-modules` in cri-tools rewrote files in your working tree, then failed if `git status` showed anything dirty. I changed them so they only check ([PR #2212](https://github.com/kubernetes-sigs/cri-tools/pull/2212), merged 2026-10-16). The PR fixes [issue #2197](https://github.com/kubernetes-sigs/cri-tools/issues/2197), which @SergeyKanzhelev filed, and it follows the plan @saschagrunert wrote in a comment there. @saschagrunert then reviewed the PR and raised four points. All four changed the merged scripts.

## What verify did before

The old `hack/verify-go-modules.sh` ran the commands that fix the tree, then asked git whether anything was left over:

```bash
go mod tidy && go mod vendor && go mod verify
# Test if we can resolve all go modules
go list -mod=readonly -m all

STATUS=$(git status --porcelain)
if [[ -z $STATUS ]]; then
    echo "tree is clean"
else
    echo "tree is dirty, please commit all changes"
    echo ""
    echo "$STATUS"
    exit 1
fi
```

`verify-docs.sh` had the same shape with `go-md2man` and `docs/crictl.1`. So a verify run edited your files, and it could not pass while you had uncommitted work. The issue puts it this way: "verify commands are both disruptive (override in-place) and cannot run on not-committed files."

## The plan I was given

@saschagrunert's [comment on the issue](https://github.com/kubernetes-sigs/cri-tools/issues/2197#issuecomment-6119691601) chose a narrow version of the third option in the issue. Build into a temp location and diff against the tree, without copying the tree. For modules that means `go mod tidy -diff`, `go mod vendor -o "$tmp"` plus a diff against `vendor`, and `go mod verify`. For docs it means running the doc tool into a temp file. Then add a `make docs` target next to `make vendor`, so that "`verify-*` only checks and `docs`/`vendor` write files".

The comment also warned that a bare entry in `.gitignore` matched a vendored file, so that file was not in git and a plain `diff -r` against `vendor/` would report it. The PR anchors that entry with a leading slash. The comment suggested committing the vendored file as well. At the version in `go.mod` the dependency ships no such file, so there was nothing to commit, and the anchor only stops a later bump from dropping one silently.

I wrote the PR to that plan. The tree stayed untouched, and in the PR description I wrote that I had checked a clean tree passes, that an edited `docs/crictl.md`, an untidy `go.mod` and an edited `vendor/modules.txt` each fail with a hint, and that the tree is byte-identical after every run. The description lists no run from another directory and none with an ignored file in the tree.

## What review changed

The review was on commit `b8c6f16`. The PR head that was merged is `9d7078f`; the PR is still one commit.

- **Run from anywhere.** My `verify-go-modules.sh` ran `diff -ru vendor "$TMP_DIR"`. @saschagrunert pointed out that `vendor` is relative to the current directory, so `cd hack && ./verify-go-modules.sh` fails with `diff: vendor: No such file or directory` and a misleading "run make vendor" hint. The old `git status` check worked from any subdirectory, so I had made it worse. My `verify-docs.sh` already had the fix, and I had not copied it into this script. The review comment carried a suggestion block, and the merged script has exactly those lines:

  ```bash
  # Root of the repository
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  cd "${REPO_ROOT}"
  ```

- **Compare what git will commit.** [This comment](https://github.com/kubernetes-sigs/cri-tools/pull/2212#discussion_r4291329961) is the one I learned most from. A diff against `vendor/` compares the files on disk, including files git ignores. @saschagrunert listed where that bites: a dependency bump can vendor a file matched by a root rule (`*.iml`, `*~`, `.*/`, `release-notes.md`) or by one of the 28 vendored `.gitignore` files. Their examples were otel ignoring `gen/`, wazero ignoring `build` and `dist`, and logrus ignoring `vendor`. Then `make vendor` writes the file and it never gets committed. In the reviewer's words: "The check then passes locally and fails in CI with a hint that `make vendor` can't fix." Anchoring the one rule the plan comment named covers one instance. A root `!/vendor/**` negation would not cover the rest, because the vendored `.gitignore` files take precedence. The reviewer's answer was a check after the diff, and the merged script contains it as written:

  ```bash
  ignored="$(cd "$TMP_DIR" && find . -type f | sed 's|^\./|vendor/|' | git -C "$REPO_ROOT" check-ignore --no-index --stdin || true)"
  if [[ -n $ignored ]]; then
      echo "vendored files are gitignored and would not be committed:" >&2
      echo "$ignored" >&2
      exit 1
  fi
  ```

  I did not have this idea. It is @saschagrunert's.

- **Do not bury the hint.** On vendor drift, `diff -ru` prints a unified diff of every changed vendored file. @saschagrunert measured it: bumping `golang.org/x/net` without re-vendoring gives about 1000 lines with `diff -ru` and 9 with `diff -rq`. "Since the fix is always `make vendor`, file names should be enough." The merged script uses `diff -rq`. `verify-docs.sh` still uses `diff -u`, because it compares one file.

- **One copy of the doc-tool call.** In my first version the Makefile `docs` target and `verify-docs.sh` each had their own `go run -C hack/tools github.com/cpuguy83/go-md2man/v2 ...` line. The review said defining it once keeps the two from drifting, and offered two ways: a variable, or "having the script take the output path and `make docs` call it". I took the second. The merged Makefile has `CRICTL_MAN ?= $(CURDIR)/docs/crictl.1`, and the script runs:

  ```bash
  make -s docs CRICTL_MAN="$TMP_DIR/crictl.1"
  ```

@saschagrunert approved with "Thanks!" and the merge bot merged the PR the same day.

## Where the review found what I had not

None of the four points is covered by the checks the description lists. Each of those asks "does this fail when it should". The review asked other questions. Where is the script run from? What will CI see that my disk does not? How much output does a failure produce? Is the doc-tool call written down once?

The second question is the hard one. A verify script that reads the working tree answers "is this tree consistent", and CI needs the answer "is this commit consistent". Those differ whenever the working tree holds anything git ignores. The same split shows up in tests. A test that reads the machine it runs on, such as a global git config, passes on the machine where it was written and fails on another. I met that in [a git-stats change for oh-my-openagent](/2026/10/16/git-diff-stats-nul-separated.html), where the CI run on Windows failed on a config path.

## Before a PR that touches a verify script

- Run it from the repo root, from a subdirectory, and once with an unrelated dirty file. It must behave the same in all three.
- Hash or `git status` the tree before and after a run. A verify script that changes anything has failed.
- Write the script's output to `mktemp -d` and remove it in a `trap ... EXIT`. Never write into the tree and diff afterwards.
- Compare against what git would commit. Run `git check-ignore --no-index` on the list of files the script wrote.
- Make a failure print the hint first and the shortest evidence second. If the fix is one command, file names are enough.
- If a script and a Makefile target both run a tool, make the script call the target.
- When replacing a check, list what the old one did that you did not mean to drop. The old `git status` check worked from any directory, and the replacement lost that.

If a verify script of yours passed on your machine and failed in CI, reply on [X](https://x.com/spicysharkco).
