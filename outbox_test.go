package sup_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/webermarci/sup"
)

func TestOutbox_Emit(t *testing.T) {
	out := &sup.Outbox[int]{}
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	received := make([]int, 0)

	wg.Add(2)

	out.Subscribe(func(ctx context.Context, val int) {
		defer wg.Done()
		mu.Lock()
		received = append(received, val)
		mu.Unlock()
	})

	out.Subscribe(func(ctx context.Context, val int) {
		defer wg.Done()
		mu.Lock()
		received = append(received, val+100)
		mu.Unlock()
	})

	out.Emit(ctx, 5)
	wg.Wait()

	if len(received) != 2 {
		t.Errorf("expected 2 messages, got %d", len(received))
	}
}

func TestOutbox_Concurrency(t *testing.T) {
	out := &sup.Outbox[string]{}
	ctx := context.Background()
	const numSubscribers = 100
	const numMessages = 10

	var wg sync.WaitGroup
	var totalReceived atomic.Int64

	for range numSubscribers {
		out.Subscribe(func(ctx context.Context, msg string) {
			defer wg.Done()
			totalReceived.Add(1)
		})
	}

	expected := int64(numSubscribers * numMessages)
	wg.Add(int(expected))

	for range numMessages {
		out.Emit(ctx, "ping")
	}

	wg.Wait()

	if totalReceived.Load() != expected {
		t.Errorf("expected %d total receipts, got %d", expected, totalReceived.Load())
	}
}
