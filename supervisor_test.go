package sup_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

type actorFunc func(context.Context) error

func (f actorFunc) Run(ctx context.Context) error { return f(ctx) }
func (f actorFunc) Name() string                  { return "test-actor" }

func TestSupervisor_Temporary(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			runs.Add(1)
			panic("fatal error")
		})),
		sup.WithPolicy(sup.Temporary),
	)

	// In Temporary policy, Run should exit cleanly even after a panic
	supervisor.Run(t.Context())

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			count := runs.Add(1)
			if count == 1 {
				return errors.New("abnormal exit")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(1*time.Millisecond),
	)

	// Should restart once then exit cleanly on nil
	supervisor.Run(t.Context())

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_MaxRestartsEscalation(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			runs.Add(1)
			return errors.New("constant fail")
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(1*time.Millisecond),
		sup.WithRestartLimit(3, time.Second),
	)

	// Run should return the terminal error when limits are exceeded
	err := supervisor.Run(t.Context())

	if runs.Load() != 4 { // Initial + 3 restarts
		t.Fatalf("expected 4 runs, got %d", runs.Load())
	}

	if err == nil || !strings.Contains(err.Error(), "exceeded max restarts") {
		t.Fatalf("expected escalation error, got: %v", err)
	}
}

func TestSupervisor_OnError(t *testing.T) {
	var capturedErr error
	var capturedActor sup.Actor

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			return errors.New("boom")
		})),
		sup.WithPolicy(sup.Temporary),
		sup.WithOnError(func(a sup.Actor, err error) {
			capturedActor = a
			capturedErr = err
		}),
	)

	supervisor.Run(t.Context())

	if capturedErr == nil || capturedErr.Error() != "boom" {
		t.Errorf("failed to capture error via OnError")
	}
	if capturedActor == nil || capturedActor.Name() != "test-actor" {
		t.Errorf("failed to capture actor identity")
	}
}

func TestSupervisor_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	supervisor := sup.NewSupervisor("sup")

	// Spawn multiple long-running actors
	for range 10 {
		supervisor.Spawn(ctx, actorFunc(func(aCtx context.Context) error {
			<-aCtx.Done()
			return nil
		}))
	}

	time.Sleep(10 * time.Millisecond)
	cancel() // Shut everything down

	_ = supervisor.Run(ctx) // This will exit immediately because ctx is canceled

	// Give the scheduler a moment to clean up
	time.Sleep(50 * time.Millisecond)

	final := runtime.NumGoroutine()
	if final > initialGoroutines+5 { // Allow a small buffer for runtime overhead
		t.Fatalf("potential leak: started with %d, ended with %d", initialGoroutines, final)
	}
}

func TestSupervisor_RestartLimit_WindowReset(t *testing.T) {
	var runs atomic.Int32

	// Limit is 2 restarts in 50ms.
	// We will fail, wait 100ms, fail again. Window should reset.
	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			n := runs.Add(1)
			if n == 3 {
				time.Sleep(100 * time.Millisecond)
			}
			if n < 5 {
				return errors.New("fail")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(1*time.Millisecond),
		sup.WithRestartLimit(2, 50*time.Millisecond),
	)

	err := supervisor.Run(t.Context())
	if err != nil {
		t.Fatalf("expected clean exit after window reset, got: %v", err)
	}

	if runs.Load() != 5 {
		t.Fatalf("expected 5 runs, got %d", runs.Load())
	}
}

func TestSupervisor_PanicStackTrace(t *testing.T) {
	var capturedErr error
	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actorFunc(func(ctx context.Context) error {
			panic("extreme failure")
		})),
		sup.WithPolicy(sup.Temporary),
		sup.WithOnError(func(a sup.Actor, err error) {
			capturedErr = err
		}),
	)

	supervisor.Run(t.Context())

	if capturedErr == nil || !strings.Contains(capturedErr.Error(), "stack") {
		t.Fatal("expected stack trace in panic recovery")
	}
}
