package bus

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockWriter is a thread-safe slice that records writes for testing
type mockWriter[V any] struct {
	values []V
	mu     sync.Mutex
}

func (m *mockWriter[V]) Write(v V) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = append(m.values, v)
	return nil
}

func (m *mockWriter[V]) getValues() []V {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]V, len(m.values))
	copy(copied, m.values)
	return copied
}

func TestLink_ForwardsValues(t *testing.T) {
	ctx := t.Context()

	// Source
	trigger := NewTrigger("src", func(ctx context.Context, v int) error { return nil })
	go trigger.Run(ctx)

	// Destination
	dest := &mockWriter[int]{}

	// Wiring
	link := NewLink("link", trigger, dest)
	go link.Run(ctx)

	// Wait for subscriptions
	time.Sleep(20 * time.Millisecond)

	// Act
	trigger.Write(ctx, 1)
	trigger.Write(ctx, 2)
	trigger.Write(ctx, 3)

	// Wait for propagation
	time.Sleep(50 * time.Millisecond)

	// Assert
	vals := dest.getValues()
	if len(vals) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(vals))
	}
	if vals[0] != 1 || vals[1] != 2 || vals[2] != 3 {
		t.Errorf("Expected [1, 2, 3], got %v", vals)
	}
}

func TestLink_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	trigger := NewTrigger("src", func(ctx context.Context, v int) error { return nil })
	go trigger.Run(ctx)

	dest := &mockWriter[int]{}
	link := NewLink("link", trigger, dest)

	// Run link in a way we can track its exit
	done := make(chan struct{})
	go func() {
		_ = link.Run(ctx)
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
		t.Fatal("Link did not shut down on context cancellation")
	}
}
