package sup_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSignal_ProcessorDebounce(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("search", "").
		Debounce(50 * time.Millisecond)

	values := signal.Subscribe(ctx)

	if err := signal.Write(ctx, "h"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	if err := signal.Write(ctx, "he"); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	if err := signal.Write(ctx, "hel"); err != nil {
		t.Fatalf("third write failed: %v", err)
	}

	select {
	case got := <-values:
		t.Fatalf("expected no immediate debounced value, got %q", got)

	case <-time.After(25 * time.Millisecond):
	}

	select {
	case got := <-values:
		if got != "hel" {
			t.Fatalf("expected debounced value %q, got %q", "hel", got)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced value")
	}

	if got := signal.Read(); got != "hel" {
		t.Fatalf("expected stored value %q, got %q", "hel", got)
	}
}

func TestSignal_DebounceResetOnRunDropsStalePendingValue(t *testing.T) {
	writeCtx := t.Context()
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("search", "").
		Debounce(20 * time.Millisecond)
	values := signal.Subscribe(runCtx)

	if err := signal.Write(writeCtx, "stale"); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- signal.Run(runCtx)
	}()

	select {
	case got := <-values:
		t.Fatalf("expected reset to drop stale pending value, got %q", got)
	case err := <-done:
		t.Fatalf("signal exited early: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
}
