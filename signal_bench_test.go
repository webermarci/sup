package sup_test

import (
	"testing"

	"github.com/webermarci/sup"
)

// BenchmarkSignal_Read measures the cost of reading the current value from the
// BaseSignal-backed storage.
func BenchmarkSignal_Read(b *testing.B) {
	signal := sup.NewSignal("count", 42)

	for b.Loop() {
		_ = signal.Read()
	}
}

// BenchmarkSignal_Write measures the cost of writing distinct values without
// any subscribers or processors attached.
func BenchmarkSignal_Write(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0)

	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignal_WriteEqual measures the short-circuit path where the equality
// function rejects the write as unchanged.
func BenchmarkSignal_WriteEqual(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0).
		Equal(func(a, b int) bool {
			return a == b
		})

	for b.Loop() {
		if err := signal.Write(ctx, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignal_WriteWithSubscriber measures write + value notification
// overhead with one active subscriber draining updates.
func BenchmarkSignal_WriteWithSubscriber(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0).
		Buffer(1024)

	values := signal.Subscribe(ctx)
	go func() {
		for range values {
		}
	}()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignal_WriteWithWatcher measures write + notification overhead with
// one active watcher draining change notifications.
func BenchmarkSignal_WriteWithWatcher(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0)

	updates := signal.Watch(ctx)
	go func() {
		for range updates {
		}
	}()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignal_ParallelWrite measures contention when multiple goroutines
// write to the same signal simultaneously.
func BenchmarkSignal_ParallelWrite(b *testing.B) {
	ctx := b.Context()
	signal := sup.NewSignal("count", 0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if err := signal.Write(ctx, i); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}
