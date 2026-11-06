# kube-router PR #2192: scaling benchmarks

Scratch benchmarks behind the post
[kube-router: a dead check that searched the filter-table buffer once per pod](https://spicyshark.co/2026/10/15/kube-router-netpol-sync-copy.html).
They were not sent upstream.

- `pod_scale_test.go`: `BenchmarkScalePods` (table fixed at 1 MiB), `BenchmarkScaleTable` (80 pods),
  `BenchmarkScaleCoupled` (table grows with pods), `BenchmarkScaleRefill` (the harness's own refill cost) and
  `TestScaleBytesPerPod`.
- `pod_split_test.go`: `BenchmarkSplit`, the old check split into its copy and its search on a 1 MiB table.

To run them, copy both files into `pkg/controllers/netpol/` of kube-router at `ecf5008a` (the merge commit). They
use `newPodFwTestController` and `podFwTestPod` from `pod_test.go` there. For the "before" numbers, put the deleted
lines back with `git show ecf5008a -- pkg/controllers/netpol/pod.go | git apply -R` in a second worktree.

```sh
go test -run '^$' -bench '^Benchmark(Scale|Split)' -benchmem -count 6 ./pkg/controllers/netpol/
```

The package builds on Linux only. The post's numbers come from six interleaved before/after rounds, compared with
`benchstat`.
