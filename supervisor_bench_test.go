package sup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/webermarci/sup"
)

type mockActor struct {
	*sup.BaseActor
	runFunc func(context.Context) error
}

func newMockActor(name string, fn func(context.Context) error) *mockActor {
	return &mockActor{BaseActor: sup.NewBaseActor(name), runFunc: fn}
}

func (m *mockActor) Run(ctx context.Context) error {
	return m.runFunc(ctx)
}

// BenchmarkSupervisor_SpawnAndExit measures the overhead of starting an actor
// that exits immediately (cleanly).
func BenchmarkSupervisor_SpawnAndExit(b *testing.B) {
	ctx := b.Context()

	s := sup.NewSupervisor("bench_sup").
		Policy(sup.Temporary)

	actor := newMockActor("churn", func(ctx context.Context) error {
		return nil
	})

	for b.Loop() {
		s.Spawn(ctx, actor)
	}
	s.Wait()
}

// BenchmarkSupervisor_RestartCycle measures the overhead of the internal
// restart loop logic (including the policy checks and jitter calculations).
func BenchmarkSupervisor_RestartCycle(b *testing.B) {
	// We want to measure how fast the supervisor can process restarts
	// when the policy is set to Permanent.
	s := sup.NewSupervisor("bench_sup").
		Policy(sup.Permanent).
		RestartDelay(0)

	// We use a counter to stop the actor after b.N restarts
	count := 0
	actor := newMockActor("restarter", func(ctx context.Context) error {
		count++
		if count >= b.N {
			// Once we hit b.N, we need a way to stop the supervisor loop
			// Since there isn't a "Self-Kill" easily accessible here without ctx,
			// we just return nil and use a context cancel elsewhere.
			return nil
		}
		return errors.New("restart me")
	})

	ctx, cancel := context.WithCancel(context.Background())

	b.ResetTimer()
	// Start the actor loop
	s.Spawn(ctx, actor)

	// We have to wait for the actor to reach the count
	for count < b.N {
		// busy wait or small sleep for the actor to churn
	}
	b.StopTimer()
	cancel()
}

// BenchmarkSupervisor_ParallelSpawn measures contention when multiple
// goroutines try to spawn actors on the same supervisor.
func BenchmarkSupervisor_ParallelSpawn(b *testing.B) {
	ctx := b.Context()
	s := sup.NewSupervisor("bench_sup").
		Policy(sup.Temporary)
	actor := newMockActor("p", func(ctx context.Context) error { return nil })

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Spawn(ctx, actor)
		}
	})
	s.Wait()
}
