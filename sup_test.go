package sup_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

type Increment struct{ Amount int }
type PanicCmd struct{}
type StopCmd struct{}
type GetCount struct{}

type CounterState struct {
	Total int
}

type CounterActor struct {
	sup.BaseActor[string, any, GetCount, CounterState]
	count int
}

func (c *CounterActor) ReceiveCast(ctx context.Context, msg any) {
	switch m := msg.(type) {
	case Increment:
		c.count += m.Amount
	case PanicCmd:
		panic("simulated fatal error")
	case StopCmd:
		c.Stop()
	}
}

func (c *CounterActor) ReceiveCall(ctx context.Context, msg GetCount) (CounterState, error) {
	return CounterState{Total: c.count}, nil
}

func NewCounter(id string) sup.Producer[string, any, GetCount, CounterState] {
	return func() sup.Actor[string, any, GetCount, CounterState] {
		return &CounterActor{
			BaseActor: sup.BaseActor[string, any, GetCount, CounterState]{ID: id},
			count:     0,
		}
	}
}

func TestActor_CastAndCall(t *testing.T) {
	ctx := t.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("counter-1"))

	state, err := ref.Call(ctx, GetCount{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if state.Total != 0 {
		t.Fatalf("expected count 0, got %d", state.Total)
	}

	ref.Cast(Increment{Amount: 5})
	ref.Cast(Increment{Amount: 10})

	time.Sleep(10 * time.Millisecond)

	state, err = ref.Call(ctx, GetCount{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if state.Total != 15 {
		t.Fatalf("expected count 15, got %d", state.Total)
	}
}

func TestActor_CrashRecovery(t *testing.T) {
	ctx := t.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("counter-recover"))

	ref.Cast(Increment{Amount: 42})
	time.Sleep(10 * time.Millisecond)

	ref.Cast(PanicCmd{})

	time.Sleep(1200 * time.Millisecond)

	state, err := ref.Call(ctx, GetCount{})
	if err != nil {
		t.Fatalf("expected actor to be alive, but got error: %v", err)
	}

	if state.Total != 0 {
		t.Fatalf("expected state to be reset to 0 after crash, got %d", state.Total)
	}

	ref.Cast(Increment{Amount: 100})
	time.Sleep(10 * time.Millisecond)

	state, _ = ref.Call(ctx, GetCount{})
	if state.Total != 100 {
		t.Fatalf("expected recovered actor to process new messages, got %d", state.Total)
	}
}

func TestActor_GracefulStop(t *testing.T) {
	ctx := t.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("counter-graceful-stop"))

	_, err := ref.Call(ctx, GetCount{})
	if err != nil {
		t.Fatalf("actor should be alive: %v", err)
	}

	ref.Cast(StopCmd{})

	time.Sleep(50 * time.Millisecond)

	ctx2, cancel2 := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel2()

	_, err = ref.Call(ctx2, GetCount{})
	if !errors.Is(err, sup.ErrProcessNotFound) {
		t.Fatalf("expected ProcessNotFound for normally stopped actor, got %v", err)
	}
}

func TestSystem_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("counter-shutdown"))

	_, err := ref.Call(ctx, GetCount{})
	if err != nil {
		t.Fatalf("actor should be alive: %v", err)
	}

	cancel()

	time.Sleep(50 * time.Millisecond)

	ctx2, cancel2 := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel2()

	_, err = ref.Call(ctx2, GetCount{})
	if err == nil {
		t.Fatal("expected error calling dead actor, got nil")
	}

	if !errors.Is(err, sup.ErrProcessNotFound) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected ProcessNotFound or Canceled, got %v", err)
	}
}

func TestActor_HighConcurrency(t *testing.T) {
	ctx := t.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("counter-stress"))

	const numGoroutines = 100
	const incrementsPerGoroutine = 1000
	const expectedTotal = numGoroutines * incrementsPerGoroutine

	var wg sync.WaitGroup

	for range numGoroutines {
		wg.Go(func() {
			for range incrementsPerGoroutine {
				ref.Cast(Increment{Amount: 1})
			}
		})
	}

	wg.Wait()

	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var finalState CounterState
	var err error

	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for actor to process all messages. got %d, expected %d", finalState.Total, expectedTotal)
		case <-ticker.C:
			finalState, err = ref.Call(ctx, GetCount{})
			if err != nil {
				t.Fatalf("failed to call actor: %v", err)
			}

			if finalState.Total == expectedTotal {
				return
			}
		}
	}
}

func BenchmarkActor_Cast(b *testing.B) {
	ctx := b.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("bench-cast"))

	msg := Increment{Amount: 1}

	b.ResetTimer()
	for b.Loop() {
		ref.Cast(msg)
	}
}

func BenchmarkActor_Call(b *testing.B) {
	ctx := b.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("bench-call"))
	msg := GetCount{}

	b.ResetTimer()
	for b.Loop() {
		_, _ = ref.Call(ctx, msg)
	}
}

func BenchmarkActor_ConcurrentCast(b *testing.B) {
	ctx := b.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("bench-concurrent-cast"))
	msg := Increment{Amount: 1}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ref.Cast(msg)
		}
	})
}

func BenchmarkActor_ConcurrentCall(b *testing.B) {
	ctx := b.Context()

	sys := sup.NewSystem(ctx)
	ref := sup.Spawn(sys, NewCounter("bench-concurrent-call"))
	msg := GetCount{}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = ref.Call(ctx, msg)
		}
	})
}
