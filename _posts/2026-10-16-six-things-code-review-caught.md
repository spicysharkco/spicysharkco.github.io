---
layout: post
title: "Six things code review caught in my upstream pull requests"
date: 2026-10-16
---

Apart from k3s #14810, which is another contributor's, I opened the pull requests below between 2026-10-05 and 2026-10-16, in a number of open-source projects, and most of them changed in review. The same few kinds of comment kept coming back. This post lists six of them, with one or two pull requests as the example for each, and ends with questions to ask before opening one. Quotes are copied from the PR threads. Where a change was the reviewer's idea, I say so.

## 1. Fix every exit path

A fix that handles the case I tested can leave another exit open. Three pull requests made this point.

[redis-py #4439](https://github.com/redis/redis-py/pull/4439) stops a reference cycle in `NodesManager.initialize()` when a later startup node answers. @petyaslavova's [review](https://github.com/redis/redis-py/pull/4439#pullrequestreview-5549297220) said: "The fix on the success path looks good, and the tests are clear. One case is still open: when no startup node answers, `initialize()` raises `... from exception` while `exception` is still bound in the frame." That is the same kind of cycle as in [an earlier post](/2026/10/15/redis-py-asyncio-error-reference-cycle.html), left on the failure path. They asked for a `try: raise ... finally: exception = None` and a test per stack, and pointed to #4195 for the idiom. I made both changes in [4f3549c](https://github.com/redis/redis-py/commit/4f3549cd912249b339b9c9ec8f2055d4b9041b27). The PR was still open on 2026-10-16. The change, abridged:

```python
try:
    raise RedisClusterUnreachableError(...) from exception
finally:
    exception = None
```

[oh-my-claudecode #4384](https://github.com/Yeachan-Heo/oh-my-claudecode/pull/4384) gives a helper process a hard lifetime. I checked the deadline at the top of its wait loops. The change-request reviews on the PR then found three more waits the deadline did not cover, one per round:

- The cleanup could wait up to 2.5 seconds for a state lock and delete files after the deadline ([review](https://github.com/Yeachan-Heo/oh-my-claudecode/pull/4384#pullrequestreview-5529096490), fixed in [73be2d1](https://github.com/Yeachan-Heo/oh-my-claudecode/commit/73be2d10a8)).
- The worker lock was taken with a retry count of 300 and no overall timeout, and the default overall timeout was infinite. With a 200 ms cap the review found the helper still alive after 751 ms ([review](https://github.com/Yeachan-Heo/oh-my-claudecode/pull/4384#pullrequestreview-5530131543), fixed in [0a76e3c](https://github.com/Yeachan-Heo/oh-my-claudecode/commit/0a76e3ce6a)).
- The process-listing probes had their own defaults, 2 seconds for `ps` on macOS and 3 for PowerShell, so a probe started near the deadline could run past it ([review](https://github.com/Yeachan-Heo/oh-my-claudecode/pull/4384#pullrequestreview-5530824053), fixed in [3031540](https://github.com/Yeachan-Heo/oh-my-claudecode/commit/303154014ef2136cbe87f3b2959dd4f0bffbc9fc)).

I had bounded the loop. The reviews asked about the helper. It merged on 2026-10-15.

[oh-my-codex #3786](https://github.com/Yeachan-Heo/oh-my-codex/pull/3786) kills a hung child that the HUD watcher started. An [inline review comment](https://github.com/Yeachan-Heo/oh-my-codex/pull/3786#discussion_r4266234795) pointed out that the child runs its own subprocess, and killing only the child leaves that one alive. The child is spawned detached, so it leads its own process group, and the fix signals the group with `process.kill(-child.pid, 'SIGKILL')` ([9149e91](https://github.com/Yeachan-Heo/oh-my-codex/commit/9149e918af)). A [second comment](https://github.com/Yeachan-Heo/oh-my-codex/pull/3786#discussion_r4266434698) asked for the same on Windows, which has no process groups, so the timer runs `taskkill /PID <pid> /T /F` ([72da4c0](https://github.com/Yeachan-Heo/oh-my-codex/commit/72da4c0)). I wrote in the PR description that I could not run the Windows branch.

## 2. Reuse the neighbour idiom

[external-dns #6805](https://github.com/kubernetes-sigs/external-dns/pull/6805) computes only the name of an ownership TXT record instead of rendering the whole record. My helper returned `(string, bool)`. @ivankatliarchuk's [inline comment](https://github.com/kubernetes-sigs/external-dns/pull/6805#discussion_r4268085098), offered as a nit: "An empty string is already a reliable sentinel, there is no other logic empty - false, non-empty - true." The head now returns a string, and the caller tests it:

```go
if txtName := im.txtRecordName(ep); txtName != "" && !txtRecordsSet.Has(txtName) {
	ep.WithProviderSpecific(providerSpecificForceUpdate, "true")
}
```

That reviewer then commented `/lgtm` on 2026-10-15. The PR was not merged when I wrote this.

In [anyio #1419](https://github.com/agronholm/anyio/pull/1419) my test module defined its own `class Sentinel` and checked a weak reference to it. @agronholm [commented](https://github.com/agronholm/anyio/pull/1419#discussion_r4286283040): "If you need a sentinel, use the built-in type or `typing_extensions.sentinel` on Python 3.14 and earlier." I did not switch to a standard sentinel. I rewrote the new tests to check the child's exception with `gc.get_referrers()`, as the other `TestRefcycles` tests do, and the class went away ([15be3ff](https://github.com/agronholm/anyio/commit/15be3ffdf8)). That PR was still open on 2026-10-16.

In [k3s #14811](https://github.com/k3s-io/k3s/pull/14811), @brandond [suggested](https://github.com/k3s-io/k3s/pull/14811#issuecomment-6199814080) dropping the `errors.WithMessage` wrap. The merged code sends the bare error with `p.errc <- err`. The rest of that PR is in [its own post](/2026/10/16/k3s-proxy-goroutine-leak.html).

## 3. Question the code you keep

[kube-router #2192](https://github.com/cloudnativelabs/kube-router/pull/2192) removes a per-pod scan of the whole iptables buffer. My first version still carried a duplicate check, backed by a set. My own description had said why earlier rules could never match. @aauren [quoted it](https://github.com/cloudnativelabs/kube-router/pull/2192#discussion_r4243179642) and wrote: "I would argue that we should just drop this check altogether, as far as I can tell this is dead code and never has the potential of actually coming back with a match, unless you see it differently?" I replied "Agreed, I don't see a way for it to match either." and removed the check, the set and the two tests that covered only them ([5764f1e](https://github.com/cloudnativelabs/kube-router/commit/5764f1e2df03f23be2923c061a36cf0d59215eee)). The numbers are in [the earlier post](/2026/10/15/kube-router-netpol-sync-copy.html).

[slam_toolbox #905](https://github.com/SteveMacenski/slam_toolbox/pull/905) removes copies of graph containers in `RemoveNodeFromGraph`. @SteveMacenski asked of the vertex-map hunk, ["What does changing this code buy us?"](https://github.com/SteveMacenski/slam_toolbox/pull/905#discussion_r4275807829) I [answered with a measurement](https://github.com/SteveMacenski/slam_toolbox/pull/905#discussion_r4276399111). With only that hunk reverted, removal took 528, 3135 and 6002 us at about 2k, 10k and 20k vertices, against 9.6, 39 and 60 us with it: "this hunk is about 88-98% of the gain." The same answer said my first description had overstated the share of the edge-vector copy, and I corrected the description. On another hunk the comment was that a container could be non-const instead of copied. `GetEdges()` returns a const reference, so I dropped the pointer copy instead ([f67f892](https://github.com/SteveMacenski/slam_toolbox/commit/f67f89272a0900f09994bf09e6ef0bb3dccfb39e)). It was approved and merged by @SteveMacenski on 2026-10-15.

## 4. Log level follows frequency

In k3s #14811 a connection reset or broken pipe was logged as `Proxy error:` at warning level. @bheemareddy's [inline comment](https://github.com/k3s-io/k3s/pull/14811#discussion_r4285138702) suggested moving it to debug to avoid noise, and I did that. Then @brandond [wrote](https://github.com/k3s-io/k3s/pull/14811#issuecomment-6200481197): "I believe my original intent not logging EOF was to just ignore normal connection closure errors. I'm OK with just adding these additional errors to that case, and not logging them even at debug level." They pushed that change, and the log-level test went with it. Merged on 2026-10-15.

A line that prints during normal teardown is not a warning. It may not need a line at all.

## 5. Look for competing PRs and sibling branches

[k3s #14810](https://github.com/k3s-io/k3s/pull/14810) and #14811 fixed the same leak, and were opened within four minutes of each other. @brandond asked on both ([#14810](https://github.com/k3s-io/k3s/pull/14810#issuecomment-6192175969), [#14811](https://github.com/k3s-io/k3s/pull/14811#issuecomment-6192184858)) that the two authors work out which approach to use. The author of #14810, @bheemareddy, [answered](https://github.com/k3s-io/k3s/pull/14810#issuecomment-6196207821) that #14811 "looks to be smaller fix and easily back portable". #14810 was closed, and @brandond [wrote](https://github.com/k3s-io/k3s/pull/14810#issuecomment-6201482323): "Thanks for the contribution! Lets go with the compromise in" #14811. Part of #14811 came from the other side. Its description says that logging connection resets and broken pipes at debug level follows @bheemareddy's review suggestion, "which matches the classification in #14810". The description still says debug level, but the merged code does not log them at all (section 4).

[opencv #30316](https://github.com/opencv/opencv/pull/30316) fixes an out-of-bounds index in `validateDisparity` on the 4.x branch. It merged on 2026-10-16. Right after, @asmorkalov [commented](https://github.com/opencv/opencv/pull/30316#issuecomment-6214604072): "Sounds like the same issue exists in 5.x. Could you submit complementary PR there?" I sent the port as [#30344](https://github.com/opencv/opencv/pull/30344), which was open on 2026-10-16. The [post about the fix](/2026/10/16/opencv-validatedisparity-out-of-bounds.html) also lists two earlier closed attempts at the same bug.

## 6. Hermetic scripts and tests

Two reviews showed me that a check can pass because of the machine it ran on.

In [cri-tools #2212](https://github.com/kubernetes-sigs/cri-tools/pull/2212), @saschagrunert found that my verify script depended on the current directory and compared files on disk, where CI compares what git would commit. In [oh-my-openagent #10133](https://github.com/code-yeongyu/oh-my-openagent/pull/10133), my test pointed `GIT_CONFIG_GLOBAL` at `os.devNull`. On Windows that is `\\.\nul`, git could not open it, and @code-yeongyu proposed an empty file in the test's own temp directory. The details are in two posts: [one on the cri-tools script](/2026/10/16/cri-tools-verify-does-not-write.html) and [one on the git-stats test](/2026/10/16/git-diff-stats-nul-separated.html).

## Questions to ask before opening a PR

- List every way the changed code can end: success, error, timeout, cancellation, "nobody answered". For each, name what is still held afterwards: a reference, a lock, a child process, a timer.
- Give each wait its own bound, not only the loop around it.
- Read the neighbouring code for the same job (sentinel, error wrapping, test style) and use that.
- For each hunk, write one line: what it buys, and the number that shows it. Delete the hunk that has no line.
- Ask how often a log line prints on a healthy system, and pick the level from the answer.
- Search open and closed PRs for the same lines, and grep the other long-lived branches for the same bug.
- Run a script or test from another directory, with none of the local machine's config, on the platform CI uses.

If you have a review comment that changed how you write a PR, reply on [X](https://x.com/spicysharkco).
