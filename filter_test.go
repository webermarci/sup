package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

func TestSignal_ProcessorFilterDropsValue(t *testing.T) {
	signal := sup.NewSignal("count", 0).
		Filter(func(v int) bool {
			return v >= 0
		})

	if err := signal.Write(t.Context(), -10); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := signal.Read(); got != 0 {
		t.Fatalf("expected value to remain 0, got %d", got)
	}
}

func TestSignal_ProcessorFilterAllowsValue(t *testing.T) {
	signal := sup.NewSignal("count", 0).
		Filter(func(v int) bool {
			return v >= 0
		})

	if err := signal.Write(t.Context(), 10); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := signal.Read(); got != 10 {
		t.Fatalf("expected value 10, got %d", got)
	}
}
