package sup

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSignalPipe_ForwardsValues(t *testing.T) {
	ctx := t.Context()

	// Source
	src := NewPushedSignal("src", func(ctx context.Context, v int) error {
		return nil
	})
	go src.Run(ctx)

	values := []int{}
	mu := sync.Mutex{}

	// Destination
	dest := NewPushedSignal("dest", func(ctx context.Context, v int) error {
		mu.Lock()
		defer mu.Unlock()
		values = append(values, v)
		return nil
	})
	go src.Run(ctx)

	// Wiring
	pipe := NewSignalPipe("pipe", src, dest)
	go pipe.Run(ctx)

	// Wait for subscriptions
	time.Sleep(20 * time.Millisecond)

	// Act
	src.Write(ctx, 1)
	src.Write(ctx, 2)
	src.Write(ctx, 3)

	// Wait for propagation
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Assert
	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}
	if values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Errorf("Expected [1, 2, 3], got %v", values)
	}
}

func TestSignalPipe_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	src := NewPushedSignal("src", func(ctx context.Context, v int) error {
		return nil
	})
	go src.Run(ctx)

	dest := NewPushedSignal("dest", func(ctx context.Context, v int) error {
		return nil
	})
	go src.Run(ctx)

	pipe := NewSignalPipe("pipe", src, dest)

	// Run pipe in a way we can track its exit
	done := make(chan struct{})
	go func() {
		_ = pipe.Run(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)

	// Cancel the context
	cancel()

	// Assert the Run loop exits
	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("pipe did not shut down on context cancellation")
	}
}
