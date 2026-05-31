package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

// BenchmarkProcessor_Map measures the overhead of the processor pipeline for a
// simple map processor that emits one transformed value per write.
func BenchmarkProcessor_Map(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0).
		Map(func(v int) int {
			return v + 1
		})

	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}
