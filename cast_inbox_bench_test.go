package sup

import (
	"context"
	"testing"
)

// BenchmarkCastInbox_SingleWorker measures the latency of casting a message
// when there is a dedicated consumer clearing the queue.
func BenchmarkCastInbox_SingleWorker(b *testing.B) {
	ctx := context.Background()
	inbox := NewCastInbox[int](128)

	// Start a "sink" goroutine to drain the inbox as fast as possible
	go func() {
		for range inbox.Receive() {
			// Do nothing, just drain
		}
	}()

	for i := 0; b.Loop(); i++ {
		err := inbox.Cast(ctx, i)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCastInbox_Parallel measures how the inbox handles concurrent
// writers (multiple goroutines calling Cast at once).
func BenchmarkCastInbox_Parallel(b *testing.B) {
	ctx := context.Background()
	inbox := NewCastInbox[int](1024)

	go func() {
		for range inbox.Receive() {
			// Drain
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = inbox.Cast(ctx, i)
			i++
		}
	})
}

// BenchmarkCastInbox_TryCast measures the overhead of the non-blocking
// path which avoids some of the select logic.
func BenchmarkCastInbox_TryCast(b *testing.B) {
	ctx := context.Background()
	inbox := NewCastInbox[int](b.N + 1) // Buffer large enough to never block

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inbox.TryCast(ctx, i)
	}
}
