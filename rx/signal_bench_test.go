package rx_test

import (
	"testing"

	"github.com/webermarci/sup/rx"
)

func BenchmarkSignal_Value(b *testing.B) {
	signal := rx.NewSignal("count", 42)

	for b.Loop() {
		_ = signal.Value()
	}
}

func BenchmarkSignal_Set(b *testing.B) {
	signal := rx.NewSignal("count", 0)

	for i := 0; b.Loop(); i++ {
		signal.Set(i)
	}
}

func BenchmarkSignal_SetWithSubscriber(b *testing.B) {
	ctx := b.Context()
	signal := rx.NewSignal("count", 0)

	values := signal.Subscribe(ctx)
	<-values
	go func() {
		for range values {
		}
	}()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		signal.Set(i)
	}
}

func BenchmarkSignal_SetWithWatcher(b *testing.B) {
	ctx := b.Context()
	signal := rx.NewSignal("count", 0)

	updates := signal.Watch(ctx)
	go func() {
		for range updates {
		}
	}()

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		signal.Set(i)
	}
}

func BenchmarkSignal_ParallelSet(b *testing.B) {
	signal := rx.NewSignal("count", 0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			signal.Set(i)
			i++
		}
	})
}
