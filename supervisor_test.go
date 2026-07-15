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

	supervisor := sup.NewSupervisor("sup").
		Policy(sup.Temporary).
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			runs.Add(1)
			panic("fatal error")
		}))

	// In Temporary policy, Run should exit cleanly even after a panic
	supervisor.Run(t.Context())

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup").
		Policy(sup.Transient).
		RestartDelay(time.Millisecond).
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			count := runs.Add(1)
			if count == 1 {
				return errors.New("abnormal exit")
			}
			return nil
		}))

	// Should restart once then exit cleanly on nil
	supervisor.Run(t.Context())

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_SmallRestartDelayDoesNotPanic(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup").
		Policy(sup.Transient).
		RestartDelay(time.Nanosecond).
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			if runs.Add(1) == 1 {
				return errors.New("retry")
			}
			return nil
		}))

	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("expected clean exit after retry, got %v", err)
	}

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_MaxRestartsEscalation(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup").
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			runs.Add(1)
			return errors.New("constant fail")
		})).
		Policy(sup.Permanent).
		RestartDelay(1*time.Millisecond).
		RestartLimit(3, time.Second)

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

	supervisor := sup.NewSupervisor("sup").
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			return errors.New("boom")
		})).
		Policy(sup.Temporary).
		OnError(func(a sup.Actor, err error) {
			capturedActor = a
			capturedErr = err
		})

	supervisor.Run(t.Context())

	if capturedErr == nil || capturedErr.Error() != "boom" {
		t.Errorf("failed to capture error via OnError")
	}

	if capturedActor == nil || capturedActor.ID() != t.Name() {
		t.Errorf("failed to capture actor identity")
	}
}

func TestSupervisor_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	supervisor := sup.NewSupervisor("sup")

	// Spawn multiple long-running actors
	for range 10 {
		supervisor.Spawn(ctx, sup.ActorFunc(t.Name(), func(aCtx context.Context) error {
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
	supervisor := sup.NewSupervisor("sup").
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			n := runs.Add(1)
			if n == 3 {
				time.Sleep(100 * time.Millisecond)
			}
			if n < 5 {
				return errors.New("fail")
			}
			return nil
		})).
		Policy(sup.Transient).
		RestartDelay(1*time.Millisecond).
		RestartLimit(2, 50*time.Millisecond)

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

	supervisor := sup.NewSupervisor("sup").
		Actor(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			panic("extreme failure")
		})).
		Policy(sup.Temporary).
		OnError(func(a sup.Actor, err error) {
			capturedErr = err
		})

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
		OnActorRegistered: func(s *sup.Supervisor, a sup.Actor) {
			registered.Add(1)
		},
		OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
			started.Add(1)
		},
		OnActorStopped: func(s *sup.Supervisor, a sup.Actor, err error) {
			stopped.Add(1)
		},
		OnActorRestarting: func(s *sup.Supervisor, a sup.Actor, restartCount int, lastErr error) {
			restarting.Add(1)
		},
		OnSupervisorTerminal: func(s *sup.Supervisor, err error) {
			terminal.Add(1)
		},
	}

	var runs atomic.Int32
	actor := sup.ActorFunc(t.Name(), func(ctx context.Context) error {
		n := runs.Add(1)
		if n == 1 {
			return errors.New("boom")
		}
		return nil
	})

	supervisor := sup.NewSupervisor("sup").
		Actor(actor).
		Policy(sup.Transient).
		RestartDelay(5 * time.Millisecond).
		Observer(observer)

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

func TestSupervisor_Inspect(t *testing.T) {
	child1 := sup.ActorFunc("child1", func(ctx context.Context) error { return nil })
	child2 := sup.ActorFunc("child2", func(ctx context.Context) error { return nil })

	s := sup.NewSupervisor("root").
		Actors(child1, child2)

	spec := s.Inspect()

	if spec.Kind != "supervisor" {
		t.Fatalf("expected kind supervisor, got %q", spec.Kind)
	}

	if got := spec.Metadata["actor_count"]; got != "2" {
		t.Fatalf("expected actor_count=2, got %q", got)
	}

	if got := spec.Metadata["restart_delay"]; got != time.Second.String() {
		t.Fatalf("expected restart_delay=%q, got %q", time.Second.String(), got)
	}

	if spec.Dependencies == nil || len(spec.Dependencies) != 0 {
		t.Fatalf("expected no dependencies for supervisor, got %v", spec.Dependencies)
	}
}

func TestSupervisor_ChildrenReturnsCopy(t *testing.T) {
	child1 := sup.ActorFunc("child1", func(ctx context.Context) error { return nil })
	child2 := sup.ActorFunc("child2", func(ctx context.Context) error { return nil })
	replacement := sup.ActorFunc("replacement", func(ctx context.Context) error { return nil })

	s := sup.NewSupervisor("root").
		Actors(child1, child2)

	children := s.Children()
	if len(children) != 2 || children[0].ID() != "child1" || children[1].ID() != "child2" {
		t.Fatalf("unexpected children: %v", children)
	}

	children[0] = replacement

	children = s.Children()
	if children[0].ID() != "child1" {
		t.Fatalf("expected Children to return a copy, got first child %q", children[0].ID())
	}
}

func TestSupervisor_WaitBlocksUntilSpawnedActorStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	stopped := make(chan struct{})

	s := sup.NewSupervisor("root")
	s.Spawn(ctx, sup.ActorFunc("worker", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil
	}))

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for actor to start")
	}

	waitDone := make(chan struct{})
	go func() {
		s.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatal("Wait returned before actor stopped")
	case <-time.After(25 * time.Millisecond):
	}

	cancel()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for actor to stop")
	}

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Wait to return")
	}
}

func TestSupervisor_ObserverPropagatesToChildSupervisor(t *testing.T) {
	var rootObservedWorkerStarted atomic.Int32

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		return nil
	})

	child := sup.NewSupervisor("child").
		Actor(worker).
		Policy(sup.Transient)

	rootObserver := &sup.SupervisorObserver{
		OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
			if a.ID() == "worker" {
				rootObservedWorkerStarted.Add(1)
			}
		},
	}

	root := sup.NewSupervisor("root").
		Observer(rootObserver).
		Actor(child).
		Policy(sup.Transient)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if rootObservedWorkerStarted.Load() == 1 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected root observer to see worker start, got %d", rootObservedWorkerStarted.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestSupervisor_ObserverPropagatesThroughSupervisorTree(t *testing.T) {
	var observedLeafWorkerStarted atomic.Int32

	worker := sup.ActorFunc("leaf-worker", func(ctx context.Context) error {
		return nil
	})

	leaf := sup.NewSupervisor("leaf").
		Actor(worker).
		Policy(sup.Transient)

	middle := sup.NewSupervisor("middle").
		Actor(leaf).
		Policy(sup.Transient)

	rootObserver := &sup.SupervisorObserver{
		OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
			if a.ID() == "leaf-worker" {
				observedLeafWorkerStarted.Add(1)
			}
		},
	}

	root := sup.NewSupervisor("root").
		Observer(rootObserver).
		Actor(middle).
		Policy(sup.Transient)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if observedLeafWorkerStarted.Load() == 1 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected root observer to see leaf worker start, got %d", observedLeafWorkerStarted.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestSupervisor_ObserverPropagationDoesNotDependOnConfigurationOrder(t *testing.T) {
	var observed atomic.Int32

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		return nil
	})

	child := sup.NewSupervisor("child").
		Actor(worker).
		Policy(sup.Transient)

	observer := &sup.SupervisorObserver{
		OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
			if a.ID() == "worker" {
				observed.Add(1)
			}
		},
	}

	root := sup.NewSupervisor("root").
		Actor(child).
		Observer(observer).
		Policy(sup.Transient)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if observed.Load() == 1 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected observer to propagate despite configuration order, got %d", observed.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestSupervisor_ObserverPropagationDoesNotDuplicateObserver(t *testing.T) {
	var observed atomic.Int32

	observer := &sup.SupervisorObserver{
		OnActorStarted: func(s *sup.Supervisor, a sup.Actor) {
			if a.ID() == "worker" {
				observed.Add(1)
			}
		},
	}

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		return nil
	})

	child := sup.NewSupervisor("child").
		Observer(observer).
		Actor(worker).
		Policy(sup.Transient)

	root := sup.NewSupervisor("root").
		Observer(observer).
		Actor(child).
		Policy(sup.Transient)

	if err := root.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if observed.Load() == 1 {
			return
		}

		if observed.Load() > 1 {
			t.Fatalf("expected observer to fire once, got %d", observed.Load())
		}

		select {
		case <-deadline:
			t.Fatalf("expected observer to fire once, got %d", observed.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
