package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

// BenchmarkCastInbox_SingleWorker measures the latency of casting a message
// when there is a dedicated consumer clearing the queue.
func BenchmarkCastInbox_SingleWorker(b *testing.B) {
	ctx := b.Context()
	inbox := sup.NewCastInbox[int](128)

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
	ctx := b.Context()
	inbox := sup.NewCastInbox[int](1024)

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
	ctx := b.Context()
	const batchSize = 1024
	inbox := sup.NewCastInbox[int](batchSize)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if i > 0 && i%batchSize == 0 {
			b.StopTimer()
			for len(inbox.Receive()) > 0 {
				<-inbox.Receive()
			}
			b.StartTimer()
		}
		if err := inbox.TryCast(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}
