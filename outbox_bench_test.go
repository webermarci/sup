package sup_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/webermarci/sup"
)

func BenchmarkOutbox_Emit(b *testing.B) {
	ctx := context.Background()
	counts := []int{1, 10, 100}

	for _, count := range counts {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			out := &sup.Outbox[int]{}
			var wg sync.WaitGroup

			for range count {
				out.Subscribe(func(ctx context.Context, val int) {
					wg.Done()
				})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				wg.Add(count)
				out.Emit(ctx, i)
				wg.Wait()
			}
		})
	}
}

func BenchmarkOutbox_Subscribe(b *testing.B) {
	out := &sup.Outbox[int]{}
	handler := func(ctx context.Context, val int) {}

	for b.Loop() {
		out.Subscribe(handler)
	}
}

func BenchmarkOutbox_FireAndForget(b *testing.B) {
	ctx := context.Background()
	out := &sup.Outbox[int]{}
	out.Subscribe(func(ctx context.Context, val int) {})

	for i := 0; b.Loop(); i++ {
		out.Emit(ctx, i)
	}
}
