package sup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkPeriodicSignal_Update(b *testing.B) {
	intervals := []time.Duration{
		time.Microsecond,
		10 * time.Microsecond,
		100 * time.Microsecond,
	}

	for _, interval := range intervals {
		b.Run(interval.String(), func(b *testing.B) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var count int64
			updates := make(chan struct{}, b.N)

			s := NewPeriodicSignal("bench-periodic", func(_ context.Context) (int, error) {
				v := atomic.AddInt64(&count, 1)
				updates <- struct{}{}
				if v >= int64(b.N) {
					cancel()
				}
				return int(v), nil
			}, interval)

			go s.Run(ctx)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				<-updates
			}
			b.StopTimer()
		})
	}
}
