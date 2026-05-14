package ui

import (
	"context"
	"testing"

	"github.com/webermarci/sup"
)

func TestDashboard_Inspect(t *testing.T) {
	p := sup.NewPushedSignal("pushed", func(ctx context.Context, v int) error { return nil })
	d := NewDashboard("dashboard", WithObserve(p))
	spec := d.Inspect()

	if spec.Kind != "dashboard" {
		t.Fatalf("expected kind dashboard, got %q", spec.Kind)
	}

	if got := spec.Metadata["observed_count"]; got != "1" {
		t.Fatalf("expected observed_count=%q, got %q", "1", got)
	}
}
