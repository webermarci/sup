package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

func TestSignal_ProcessorMap(t *testing.T) {
	signal := sup.NewSignal("count", 0).
		Map(func(v int) int {
			return v * 2
		})

	if err := signal.Write(t.Context(), 10); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := signal.Read(); got != 20 {
		t.Fatalf("expected mapped value 20, got %d", got)
	}
}
