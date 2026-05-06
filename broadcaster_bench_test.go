package sup

import (
	"fmt"
	"testing"
)

func BenchmarkBroadcaster_Notify(b *testing.B) {
	for _, subs := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("%d", subs), func(b *testing.B) {
			var bc broadcaster[int]
			bc.buffer = 16
			ctx := b.Context()

			for range subs {
				ch := bc.subscribeValues(ctx, 0, false)
				go func(c <-chan int) {
					for range c {
					}
				}(ch)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bc.notify(i)
			}
		})
	}
}
