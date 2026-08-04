package sup_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

type emptyIDActor struct{}

func (emptyIDActor) ID() string                { return "" }
func (emptyIDActor) Run(context.Context) error { return nil }

func TestSupervisor_Temporary(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithPolicy(sup.Temporary),
		sup.WithActors(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			runs.Add(1)
			panic("fatal error")
		})),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup",
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(time.Millisecond),
		sup.WithActors(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			count := runs.Add(1)
			if count == 1 {
				return errors.New("abnormal exit")
			}
			return nil
		})),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if runs.Load() != 2 {
		t.Fatalf("expected 2 runs, got %d", runs.Load())
	}
}

func TestSupervisor_PermanentRestartsCleanExit(t *testing.T) {
	var runs atomic.Int32
	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(sup.ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return nil
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(0),
		sup.WithRestartLimit(2, time.Second),
	)

	if err := supervisor.Run(t.Context()); err == nil {
		t.Fatal("expected restart limit error")
	}
	if runs.Load() != 3 {
		t.Fatalf("expected initial run and two restarts, got %d runs", runs.Load())
	}
}

func TestSupervisor_CancellationDuringRestartDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var runs atomic.Int32
	restarting := make(chan struct{})
	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(sup.ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return errors.New("fail")
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(time.Hour),
		sup.WithEventSink(sup.EventSinkFunc(func(event sup.Event) {
			if event.Type == sup.EventActorRestarting {
				close(restarting)
			}
		})),
	)

	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()
	<-restarting
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
	if runs.Load() != 1 {
		t.Fatalf("expected no second attempt, got %d runs", runs.Load())
	}
}

func TestSupervisor_RestartLimit(t *testing.T) {
	var runs atomic.Int32
	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(sup.ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return errors.New("fail")
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(0),
		sup.WithRestartLimit(3, time.Second),
	)

	err := supervisor.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected restart limit error, got %v", err)
	}
	if runs.Load() != 4 {
		t.Fatalf("expected initial run and three restarts, got %d runs", runs.Load())
	}
}

func TestSupervisor_RestartDelayFuncReceivesRestartCount(t *testing.T) {
	var runs atomic.Int32
	var mu sync.Mutex
	var counts []int
	actor := sup.ActorFunc("worker", func(context.Context) error {
		if runs.Add(1) <= 2 {
			return errors.New("fail")
		}
		return nil
	})

	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelayFunc(func(restartCount int) time.Duration {
			mu.Lock()
			counts = append(counts, restartCount)
			mu.Unlock()
			return 0
		}),
	)

	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(counts, []int{1, 2}) {
		t.Fatalf("expected restart counts [1 2], got %v", counts)
	}
}

func TestSupervisor_RestartLimitWindowResets(t *testing.T) {
	var runs atomic.Int32
	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(sup.ActorFunc("worker", func(context.Context) error {
			run := runs.Add(1)
			if run == 3 {
				time.Sleep(50 * time.Millisecond)
			}
			if run < 5 {
				return errors.New("fail")
			}
			return nil
		})),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(0),
		sup.WithRestartLimit(2, 20*time.Millisecond),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("expected restart window to reset, got %v", err)
	}
	if runs.Load() != 5 {
		t.Fatalf("expected five runs, got %d", runs.Load())
	}
}

func TestSupervisor_PanicStackTrace(t *testing.T) {
	var capturedErr error

	supervisor := sup.NewSupervisor("sup",
		sup.WithActors(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			panic("extreme failure")
		})),
		sup.WithPolicy(sup.Temporary),
		sup.WithEventSink(sup.EventSinkFunc(func(event sup.Event) {
			if event.Type == sup.EventActorStopped {
				capturedErr = event.Err
			}
		})),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if capturedErr == nil || !strings.Contains(capturedErr.Error(), "stack") {
		t.Fatal("expected stack trace in panic recovery")
	}
}

func TestSupervisor_EventSinkReceivesRichEvents(t *testing.T) {
	boom := errors.New("boom")
	var runs atomic.Int32
	actor := sup.ActorFunc("worker", func(ctx context.Context) error {
		if runs.Add(1) == 1 {
			return boom
		}
		return nil
	})

	var mu sync.Mutex
	var events []sup.Event
	sink := sup.EventSinkFunc(func(event sup.Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Transient),
		sup.WithRestartDelay(0),
		sup.WithEventSink(sink),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	wantTypes := []sup.EventType{
		sup.EventActorRegistered,
		sup.EventActorStarted,
		sup.EventActorStopped,
		sup.EventActorRestarting,
		sup.EventActorStarted,
		sup.EventActorStopped,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("expected %d events, got %d", len(wantTypes), len(events))
	}

	for i, event := range events {
		if event.Type != wantTypes[i] {
			t.Fatalf("event %d: expected type %q, got %q", i, wantTypes[i], event.Type)
		}
		if event.Time.IsZero() {
			t.Fatalf("event %d: expected runtime timestamp", i)
		}
		if event.Actor != actor || event.Supervisor != supervisor {
			t.Fatalf("event %d: expected runtime actor and supervisor references", i)
		}
	}

	if !errors.Is(events[2].Err, boom) || !errors.Is(events[3].Err, boom) {
		t.Fatal("expected stop and restart events to retain the actor error")
	}
	if events[3].RestartCount != 1 {
		t.Fatalf("expected restart count 1, got %d", events[3].RestartCount)
	}
}

func TestSupervisor_EventSinkPanicDoesNotInterruptSupervision(t *testing.T) {
	var received atomic.Int32
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })

	supervisor := sup.NewSupervisor("root",
		sup.WithActors(actor),
		sup.WithPolicy(sup.Temporary),
		sup.WithEventSink(
			sup.EventSinkFunc(func(sup.Event) { panic("sink failure") }),
			sup.EventSinkFunc(func(sup.Event) { received.Add(1) }),
		),
	)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if received.Load() != 3 {
		t.Fatalf("expected second event sink to receive all 3 events, got %d", received.Load())
	}
}

func TestSupervisor_Inspect(t *testing.T) {
	child1 := sup.ActorFunc("child1", func(ctx context.Context) error { return nil })
	child2 := sup.ActorFunc("child2", func(ctx context.Context) error { return nil })

	s := sup.NewSupervisor("root", sup.WithActors(child1, child2))

	spec := s.Inspect()

	if spec.Kind != "supervisor" {
		t.Fatalf("expected kind supervisor, got %q", spec.Kind)
	}

	if got := spec.Metadata["actor_count"]; got != 2 {
		t.Fatalf("expected actor_count=2, got %v", got)
	}

	if got := spec.Metadata["restart_delay"]; got != time.Second {
		t.Fatalf("expected restart_delay=%v, got %v", time.Second, got)
	}

	if spec.Dependencies == nil || len(spec.Dependencies) != 0 {
		t.Fatalf("expected no dependencies for supervisor, got %v", spec.Dependencies)
	}
}

func TestSupervisorRejectsInvalidConfiguration(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })

	requirePanic(t, func() { sup.NewSupervisor("") })
	requirePanic(t, func() { sup.NewSupervisor("root", nil) })
	requirePanic(t, func() { sup.NewSupervisor("root", sup.WithActors(nil)) })
	requirePanic(t, func() { sup.NewSupervisor("root", sup.WithActors(emptyIDActor{})) })
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithActors(actor, actor))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithActors(actor), sup.WithActors(actor))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithPolicy(sup.RestartPolicy(255)))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithRestartDelay(-time.Nanosecond))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithRestartDelayFunc(nil))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithRestartLimit(0, time.Second))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithRestartLimit(1, 0))
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.WithEventSink(nil))
	})
}

func TestSupervisor_EventSinkPropagatesThroughSupervisorTree(t *testing.T) {
	var observedLeafWorkerStarted atomic.Int32

	worker := sup.ActorFunc("leaf-worker", func(ctx context.Context) error {
		return nil
	})

	leaf := sup.NewSupervisor("leaf",
		sup.WithActors(worker),
		sup.WithPolicy(sup.Transient),
	)

	middle := sup.NewSupervisor("middle",
		sup.WithActors(leaf),
		sup.WithPolicy(sup.Transient),
	)

	rootSink := sup.EventSinkFunc(func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor != nil && event.Actor.ID() == "leaf-worker" {
			observedLeafWorkerStarted.Add(1)
		}
	})

	root := sup.NewSupervisor("root",
		sup.WithEventSink(rootSink),
		sup.WithActors(middle),
		sup.WithPolicy(sup.Transient),
	)

	if err := runSupervisorUntil(t, root, func() bool {
		return observedLeafWorkerStarted.Load() == 1
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if observedLeafWorkerStarted.Load() != 1 {
		t.Fatalf("expected root sink to see leaf worker start, got %d", observedLeafWorkerStarted.Load())
	}
}

func TestSupervisor_LocalAndParentEventSinksReceiveChildEvents(t *testing.T) {
	var parentObserved atomic.Int32
	var childObserved atomic.Int32

	parentSink := sup.EventSinkFunc(func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor != nil && event.Actor.ID() == "worker" {
			parentObserved.Add(1)
		}
	})
	childSink := sup.EventSinkFunc(func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor != nil && event.Actor.ID() == "worker" {
			childObserved.Add(1)
		}
	})

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		return nil
	})

	child := sup.NewSupervisor("child",
		sup.WithEventSink(childSink),
		sup.WithActors(worker),
		sup.WithPolicy(sup.Transient),
	)

	root := sup.NewSupervisor("root",
		sup.WithEventSink(parentSink),
		sup.WithActors(child),
		sup.WithPolicy(sup.Transient),
	)

	if err := runSupervisorUntil(t, root, func() bool {
		return parentObserved.Load() == 1 && childObserved.Load() == 1
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if parentObserved.Load() != 1 || childObserved.Load() != 1 {
		t.Fatalf("expected both sinks to receive one event, parent=%d child=%d",
			parentObserved.Load(), childObserved.Load())
	}
}

func runUntilSupervisorEmpty(t *testing.T, supervisor *sup.Supervisor) error {
	t.Helper()
	return supervisor.Run(t.Context())
}

func runSupervisorUntil(
	t *testing.T,
	supervisor *sup.Supervisor,
	condition func() bool,
) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case err := <-done:
			if !condition() {
				t.Fatalf("supervisor stopped before condition was met: %v", err)
			}
			return err
		case <-deadline.C:
			t.Fatal("timed out waiting for supervisor condition")
		case <-ticker.C:
		}
	}

	cancel()
	return <-done
}
