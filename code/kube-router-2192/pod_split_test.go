package netpol

import (
	"bytes"
	"strings"
	"testing"

	api "k8s.io/api/core/v1"
)

// Scratch: split the old per-pod check into its copy and its scan, on a 1 MiB table. Not for upstream.

var (
	splitSinkS string
	splitSinkB bool
)

func splitNeedle(b *testing.B) string {
	npc := newPodFwTestController(b, []api.IPFamily{api.IPv4Protocol}, scalePods(1)...)
	buf := npc.filterTableRules[api.IPv4Protocol]
	buf.Reset()
	npc.syncPodFirewallChains(nil, "v")
	for _, l := range strings.SplitAfter(buf.String(), "\n") {
		if strings.Contains(l, "NFLOG") {
			return l
		}
	}
	b.Fatal("no NFLOG rule")
	return ""
}

func BenchmarkSplit(b *testing.B) {
	needle := splitNeedle(b)
	b.Logf("needle %d bytes: %q", len(needle), needle)
	var table bytes.Buffer
	table.Write(scaleFiller(1 << 20))
	s := table.String()
	// Same length, first byte never in the table: IndexByte skips straight to the end.
	rare := "#" + needle[1:]

	b.Run("copy=String()", func(b *testing.B) {
		for b.Loop() {
			splitSinkS = table.String()
		}
	})
	b.Run("scan=strings.Contains", func(b *testing.B) {
		for b.Loop() {
			splitSinkB = strings.Contains(s, needle)
		}
	})
	b.Run("both=old-check", func(b *testing.B) {
		for b.Loop() {
			splitSinkB = strings.Contains(table.String(), needle)
		}
	})
	b.Run("scan=bytes.Contains-no-copy", func(b *testing.B) {
		nb := []byte(needle)
		for b.Loop() {
			splitSinkB = bytes.Contains(table.Bytes(), nb)
		}
	})
	b.Run("scan=rare-first-byte", func(b *testing.B) {
		for b.Loop() {
			splitSinkB = strings.Contains(s, rare)
		}
	})
}
