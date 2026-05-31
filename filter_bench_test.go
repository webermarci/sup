package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

// BenchmarkProcessor_FilterPass measures the processor pipeline when the filter
// accepts each value and emits it downstream.
func BenchmarkProcessor_FilterPass(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0).
		Filter(func(v int) bool {
			return true
		})

	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcessor_FilterReject measures the processor pipeline when the
// filter rejects each value and does not emit downstream.
func BenchmarkProcessor_FilterReject(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0).
		Filter(func(v int) bool {
			return false
		})

	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}
