package sup_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

// BenchmarkEffect_OnSignalUpdate measures end-to-end delivery from a signal
// write through the effect's subscription loop to the effect callback.
func BenchmarkEffect_OnSignalUpdate(b *testing.B) {
	ctx := b.Context()

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	values := make(chan int)

	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		values <- value
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	select {
	case got := <-values:
		if got != 0 {
			b.Fatalf("expected initial value 0, got %d", got)
		}

	case err := <-done:
		b.Fatalf("effect exited early: %v", err)

	case <-time.After(time.Second):
		b.Fatal("timed out waiting for initial effect")
	}

	for i := 0; b.Loop(); i++ {
		if err := signal.Write(ctx, i); err != nil {
			b.Fatal(err)
		}

		<-values
	}
}
