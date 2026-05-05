package sup_test

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSupervisor_Temporary(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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

	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected escalation error, got: %v", err)
	}
}

func TestSupervisor_OnError(t *testing.T) {
	var capturedErr error
	var capturedActor sup.Actor

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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

	if capturedActor == nil || capturedActor.Name() != t.Name() {
		t.Errorf("failed to capture actor identity")
	}
}

func TestSupervisor_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	supervisor := sup.NewSupervisor("sup")

	// Spawn multiple long-running actors
	for range 10 {
		supervisor.Spawn(ctx, sup.ActorFunc(t.Name(), func(aCtx context.Context, _ *slog.Logger) error {
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
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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
		sup.WithActor(sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
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

func TestSupervisor_ObserverBasicLifecycle(t *testing.T) {
	var registered atomic.Int32
	var started atomic.Int32
	var stopped atomic.Int32
	var restarting atomic.Int32
	var terminal atomic.Int32

	observer := &sup.SupervisorObserver{
		OnActorRegistered: func(a sup.Actor) {
			registered.Add(1)
		},
		OnActorStarted: func(a sup.Actor) {
			started.Add(1)
		},
		OnActorStopped: func(a sup.Actor, err error) {
			stopped.Add(1)
		},
		OnActorRestarting: func(a sup.Actor, restartCount int, lastErr error) {
			restarting.Add(1)
		},
		OnSupervisorTerminal: func(err error) {
			terminal.Add(1)
		},
	}

	var runs atomic.Int32
	actor := sup.ActorFunc(t.Name(), func(ctx context.Context, _ *slog.Logger) error {
		n := runs.Add(1)
		if n == 1 {
			return errors.New("boom")
		}
		return nil
	})

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(actor),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(5*time.Millisecond),
		sup.WithObserver(observer),
	)

	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if registered.Load() == 1 && started.Load() == 2 && stopped.Load() == 2 && restarting.Load() == 1 && terminal.Load() == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("unexpected observer counts: registered=%d started=%d stopped=%d restarted=%d terminal=%d",
				registered.Load(), started.Load(), stopped.Load(), restarting.Load(), terminal.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
