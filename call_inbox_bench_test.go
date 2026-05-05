package sup

import (
	"testing"
)

// BenchmarkCallInbox_SingleWorker measures the overhead of a single caller
// interacting with a single actor.
func BenchmarkCallInbox_SingleWorker(b *testing.B) {
	ctx := b.Context()

	inbox := NewCallInbox[int, int](128)

	go func() {
		for req := range inbox.Receive() {
			req.Reply(req.Payload()+1, nil)
		}
	}()

	for i := 0; b.Loop(); i++ {
		_, _ = inbox.Call(ctx, i)
	}
}

// BenchmarkCallInbox_Contention measures performance when multiple goroutines
// are trying to send messages to the same inbox simultaneously.
func BenchmarkCallInbox_Contention(b *testing.B) {
	ctx := b.Context()

	inbox := NewCallInbox[int, int](1024)

	go func() {
		for req := range inbox.Receive() {
			req.Reply(req.Payload()+1, nil)
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = inbox.Call(ctx, i)
			i++
		}
	})
}
