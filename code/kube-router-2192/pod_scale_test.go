package netpol

import (
	"bytes"
	"fmt"
	"testing"

	api "k8s.io/api/core/v1"
)

// Scratch scaling benchmarks for the spicyshark.co kube-router analysis. Not for upstream.

func scaleFiller(size int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < size; i++ {
		fmt.Fprintf(&b, "-A KUBE-NWPLCY-%016X -m comment --comment \"ns-%d/policy-%d ingress pods\" "+
			"-m set --match-set KUBE-SRC-%016X src -j MARK --set-xmark 0x10000/0x10000\n", i, i%40, i, i)
	}
	return b.Bytes()
}

func scalePods(n int) []*api.Pod {
	pods := make([]*api.Pod, 0, n)
	for i := range n {
		pods = append(pods, podFwTestPod("ns", fmt.Sprintf("pod-%d", i), fmt.Sprintf("10.1.%d.%d", i/200, i%200+1)))
	}
	return pods
}

// prevSyncChains returns the pod firewall chains a previous sync (another version) wrote for these pods,
// standing in for what iptables-save prints for them at the start of the next sync.
func prevSyncChains(b *testing.B, podCount int) []byte {
	npc := newPodFwTestController(b, []api.IPFamily{api.IPv4Protocol}, scalePods(podCount)...)
	buf := npc.filterTableRules[api.IPv4Protocol]
	buf.Reset()
	npc.syncPodFirewallChains(nil, "prev")
	return bytes.Clone(buf.Bytes())
}

func runScale(b *testing.B, podCount int, existing []byte) {
	npc := newPodFwTestController(b, []api.IPFamily{api.IPv4Protocol}, scalePods(podCount)...)
	buf := npc.filterTableRules[api.IPv4Protocol]
	b.ReportMetric(float64(len(existing)), "existing-B")
	for b.Loop() {
		buf.Reset()
		buf.Write(existing)
		npc.syncPodFirewallChains(nil, "bench")
	}
}

func runRefill(b *testing.B, existing []byte) {
	var buf bytes.Buffer
	buf.Grow(2 * len(existing))
	for b.Loop() {
		buf.Reset()
		buf.Write(existing)
	}
}

var scalePodCounts = []int{10, 25, 50, 80, 110, 250}

// Pods vary, table fixed at 1 MiB.
func BenchmarkScalePods(b *testing.B) {
	existing := scaleFiller(1 << 20)
	for _, n := range scalePodCounts {
		b.Run(fmt.Sprintf("pods=%d", n), func(b *testing.B) { runScale(b, n, existing) })
	}
}

// Table varies, pods fixed at 80.
func BenchmarkScaleTable(b *testing.B) {
	for _, kib := range []int{256, 1024, 4096, 16384} {
		existing := scaleFiller(kib << 10)
		b.Run(fmt.Sprintf("kib=%d", kib), func(b *testing.B) { runScale(b, 80, existing) })
	}
}

// Table grows with pods: 256 KiB of policy rules plus the previous sync's pod chains.
func BenchmarkScaleCoupled(b *testing.B) {
	base := scaleFiller(256 << 10)
	for _, n := range scalePodCounts {
		existing := append(bytes.Clone(base), prevSyncChains(b, n)...)
		b.Run(fmt.Sprintf("pods=%d", n), func(b *testing.B) { runScale(b, n, existing) })
	}
}

// Cost of the benchmark loop's own reset and refill, to subtract from the numbers above.
func BenchmarkScaleRefill(b *testing.B) {
	for _, kib := range []int{256, 1024, 4096, 16384} {
		existing := scaleFiller(kib << 10)
		b.Run(fmt.Sprintf("kib=%d", kib), func(b *testing.B) { runRefill(b, existing) })
	}
}

// Bytes one sync writes per pod, to size the coupled model.
func TestScaleBytesPerPod(t *testing.T) {
	for _, n := range []int{10, 110} {
		npc := newPodFwTestController(t, []api.IPFamily{api.IPv4Protocol}, scalePods(n)...)
		buf := npc.filterTableRules[api.IPv4Protocol]
		buf.Reset()
		npc.syncPodFirewallChains(nil, "v")
		t.Logf("pods=%d bytes=%d per-pod=%d", n, buf.Len(), buf.Len()/n)
	}
}
