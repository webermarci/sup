package sup_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/webermarci/sup"
)

func BenchmarkSupervisor_RunAndCancel(b *testing.B) {
	started := make(chan struct{})
	actor := sup.ActorFunc("worker", func(ctx context.Context) error {
		started <- struct{}{}
		<-ctx.Done()
		return nil
	})
	supervisor := sup.NewSupervisor("bench",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Temporary),
	)

	for b.Loop() {
		ctx, cancel := context.WithCancel(b.Context())
		go func() {
			<-started
			cancel()
		}()
		if err := supervisor.Run(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSupervisor_RestartCycle(b *testing.B) {
	var runs atomic.Int64
	ctx, cancel := context.WithCancel(b.Context())
	actor := sup.ActorFunc("worker", func(context.Context) error {
		if runs.Add(1) >= int64(b.N) {
			cancel()
		}
		return errors.New("restart")
	})
	supervisor := sup.NewSupervisor("bench",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(0),
	)

	b.ResetTimer()
	if err := supervisor.Run(ctx); err != nil {
		b.Fatal(err)
	}
}
