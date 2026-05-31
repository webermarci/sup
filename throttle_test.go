package sup_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSignal_ProcessorThrottle(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("count", 0).
		Throttle(50 * time.Millisecond)

	values := signal.Subscribe(ctx)

	if err := signal.Write(ctx, 1); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 1 {
			t.Fatalf("expected immediate throttled value 1, got %d", got)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first throttled value")
	}

	if err := signal.Write(ctx, 2); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	if err := signal.Write(ctx, 3); err != nil {
		t.Fatalf("third write failed: %v", err)
	}

	select {
	case got := <-values:
		t.Fatalf("expected no immediate second throttled value, got %d", got)

	case <-time.After(25 * time.Millisecond):
	}

	select {
	case got := <-values:
		if got != 3 {
			t.Fatalf("expected trailing throttled value 3, got %d", got)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for trailing throttled value")
	}

	if got := signal.Read(); got != 3 {
		t.Fatalf("expected stored value 3, got %d", got)
	}
}

func TestSignal_ThrottleResetOnRunDropsStalePendingValue(t *testing.T) {
	writeCtx := t.Context()
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		Throttle(20 * time.Millisecond)
	values := signal.Subscribe(runCtx)

	if err := signal.Write(writeCtx, 1); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	select {
	case got := <-values:
		if got != 1 {
			t.Fatalf("expected immediate value 1, got %d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for immediate throttled value")
	}

	if err := signal.Write(writeCtx, 2); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- signal.Run(runCtx)
	}()

	select {
	case got := <-values:
		t.Fatalf("expected reset to drop stale pending value, got %d", got)
	case err := <-done:
		t.Fatalf("signal exited early: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
}
