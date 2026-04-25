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

func TestSupervisor_Temporary(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			runs.Add(1)
			panic("fatal error")
		})),
		sup.WithPolicy(sup.Temporary),
	)

	if supervisor.Running() != 0 {
		t.Fatalf("expected 0 running actors, got %d", supervisor.Running())
	}

	supervisor.Run(t.Context())
	supervisor.Wait()

	if supervisor.Running() != 0 {
		t.Fatalf("expected 0 running actors after wait, got %d", supervisor.Running())
	}

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			count := runs.Add(1)
			if count == 1 {
				return errors.New("abnormal exit")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(5*time.Millisecond),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_PermanentAndPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(actorCtx context.Context) error {
			count := runs.Add(1)
			if count < 3 {
				panic("simulated panic")
			}
			<-actorCtx.Done()
			return actorCtx.Err()
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(5*time.Millisecond),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- supervisor.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("supervisor did not shut down in time")
	}

	if runs.Load() < 3 {
		t.Fatalf("expected at least 3 runs before shutdown, got %d", runs.Load())
	}
}

func TestSupervisor_OnRestart(t *testing.T) {
	var runs, restarts atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			count := runs.Add(1)
			if count < 3 {
				return errors.New("failure")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(5*time.Millisecond),
		sup.WithOnRestart(func() { restarts.Add(1) }),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if restarts.Load() != 2 {
		t.Fatalf("expected 2 restarts, got %d", restarts.Load())
	}
}

func TestSupervisor_MaxRestarts(t *testing.T) {
	var runs, errorCount atomic.Int32
	var maxRestartsReported atomic.Bool

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			runs.Add(1)
			return errors.New("continuous failure")
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(2*time.Millisecond),
		sup.WithRestartLimit(3, time.Second),
		sup.WithOnError(func(err error) {
			errorCount.Add(1)
			if errors.Is(err, sup.ErrMaxRestartsExceeded) {
				maxRestartsReported.Store(true)
			}
		}),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if runs.Load() != 4 {
		t.Fatalf("expected exactly 4 runs, got %d", runs.Load())
	}

	if errorCount.Load() != 5 {
		t.Fatalf("expected 5 OnError calls, got %d", errorCount.Load())
	}

	if !maxRestartsReported.Load() {
		t.Fatal("expected OnError to be called with ErrMaxRestartsExceeded")
	}
}

func TestSupervisor_OnError_NotCalledOnCleanExit(t *testing.T) {
	var called atomic.Bool

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error { return nil })),
		sup.WithPolicy(sup.Transient),
		sup.WithOnError(func(err error) { called.Store(true) }),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if called.Load() {
		t.Fatal("OnError should not be called on clean exit")
	}
}

func TestSupervisor_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(t.Context())

	supervisor := sup.NewSupervisor(sup.WithPolicy(sup.Permanent))

	for range 100 {
		supervisor.Spawn(ctx, sup.ActorFunc(func(actorCtx context.Context) error {
			<-actorCtx.Done()
			return actorCtx.Err()
		}))
	}

	time.Sleep(10 * time.Millisecond)
	if runtime.NumGoroutine() <= initialGoroutines {
		t.Fatal("expected goroutines to increase")
	}

	cancel()
	supervisor.Wait()

	time.Sleep(10 * time.Millisecond)

	if final := runtime.NumGoroutine(); final > initialGoroutines+5 {
		t.Fatalf("goroutine leak detected: started with %d, ended with %d", initialGoroutines, final)
	}
}

func TestSupervisor_PanicIncludesStackTrace(t *testing.T) {
	var capturedErr atomic.Value

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			panic("something exploded")
		})),
		sup.WithPolicy(sup.Temporary),
		sup.WithOnError(func(err error) { capturedErr.Store(err) }),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	err, ok := capturedErr.Load().(error)
	if !ok || err == nil {
		t.Fatal("expected OnError to be called with a non-nil error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "something exploded") {
		t.Fatalf("expected panic value in error, got: %s", msg)
	}
	if !strings.Contains(msg, "goroutine") {
		t.Fatalf("expected stack trace in error, got: %s", msg)
	}
}

func TestSupervisor_Running_ReflectsActiveCount(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ready := make(chan struct{})

	supervisor := sup.NewSupervisor(sup.WithPolicy(sup.Permanent))

	for range 3 {
		supervisor.Spawn(ctx, sup.ActorFunc(func(ctx context.Context) error {
			ready <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}))
	}

	for range 3 {
		<-ready
	}

	if supervisor.Running() != 3 {
		t.Fatalf("expected 3 running, got %d", supervisor.Running())
	}
}

func TestSupervisor_Running_MixedCount(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	actor := sup.ActorFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	s := sup.NewSupervisor(sup.WithActor(actor))
	go s.Run(ctx)
	s.Spawn(ctx, actor)
	s.Spawn(ctx, actor)

	time.Sleep(10 * time.Millisecond)

	if s.Running() != 3 {
		t.Fatalf("expected 3 running actors, got %d", s.Running())
	}
}

func TestSupervisor_RecursiveShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var childStarted atomic.Bool

	childSup := sup.NewSupervisor(sup.WithActor(sup.ActorFunc(func(actorCtx context.Context) error {
		childStarted.Store(true)
		<-actorCtx.Done()
		return nil
	})))

	root := sup.NewSupervisor(sup.WithActor(childSup))

	errCh := make(chan error, 1)
	go func() { errCh <- root.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	if !childStarted.Load() {
		t.Fatal("nested child actor did not start")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected error on shutdown: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("hierarchical shutdown timed out")
	}
}

func TestSupervisor_Transient_CleanExitDoesNotRestart(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			runs.Add(1)
			return nil // clean exit — must not restart
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(5*time.Millisecond),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if runs.Load() != 1 {
		t.Fatalf("expected exactly 1 run on clean exit, got %d", runs.Load())
	}
}

func TestSupervisor_Temporary_ErrorDoesNotRestart(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			runs.Add(1)
			return errors.New("some error") // Temporary must not restart even on error
		})),
		sup.WithPolicy(sup.Temporary),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	if runs.Load() != 1 {
		t.Fatalf("expected exactly 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_RestartDelay(t *testing.T) {
	var runs atomic.Int32
	delay := 50 * time.Millisecond
	start := time.Now()

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			runs.Add(1)
			if runs.Load() < 3 {
				return errors.New("failure")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(delay),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	elapsed := time.Since(start)
	// 2 restarts × 50ms minimum
	if elapsed < 2*delay {
		t.Fatalf("expected at least %v elapsed for 2 restarts, got %v", 2*delay, elapsed)
	}
}

func TestSupervisor_RestartLimit_WindowReset(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(ctx context.Context) error {
			n := runs.Add(1)
			if n == 3 {
				// Outlast the 20ms window so t1 and t2 expire before this run returns.
				// When the window check runs after this, restarts resets to [t3_end].
				time.Sleep(100 * time.Millisecond)
			}
			return errors.New("failure")
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(1*time.Millisecond),
		sup.WithRestartLimit(2, 20*time.Millisecond),
	)

	supervisor.Run(t.Context())
	supervisor.Wait()

	// First window:  runs 1, 2 → 2 restarts
	// Run 3 sleeps 100ms → t1, t2 expire → counter resets to 1
	// Second window: runs 4, 5 → limit hit → stop
	// Total: 5 runs
	if runs.Load() != 5 {
		t.Fatalf("expected 5 runs across two windows, got %d", runs.Load())
	}
}

func TestSupervisor_Run_ReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	supervisor := sup.NewSupervisor(
		sup.WithActor(sup.ActorFunc(func(actorCtx context.Context) error {
			<-actorCtx.Done()
			return actorCtx.Err()
		})),
		sup.WithPolicy(sup.Permanent),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- supervisor.Run(ctx) }()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("supervisor did not return after context cancel")
	}
}
