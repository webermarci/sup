package sup_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSignal_SourcePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	next := 0

	signal := sup.NewSignal("count", 0).
		Poll(10*time.Millisecond, func(ctx context.Context) (int, error) {
			next++
			return next, nil
		})

	done := make(chan error, 1)
	go func() {
		done <- signal.Run(ctx)
	}()

	values := signal.Subscribe(ctx)

	select {
	case got := <-values:
		if got != 1 {
			t.Fatalf("expected first source value 1, got %d", got)
		}

	case err := <-done:
		t.Fatalf("signal exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for source update")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal shutdown")
	}
}
