package sup_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/webermarci/sup"
)

type emptyIDActor struct{}

func (emptyIDActor) ID() string                { return "" }
func (emptyIDActor) Run(context.Context) error { return nil }

type eventHandlerActor struct {
	id     string
	handle func(sup.Event)
}

func newEventHandlerActor(id string, handle func(sup.Event)) *eventHandlerActor {
	return &eventHandlerActor{id: id, handle: handle}
}

func (a *eventHandlerActor) ID() string                { return a.id }
func (a *eventHandlerActor) Run(context.Context) error { return nil }
func (a *eventHandlerActor) HandleEvent(event sup.Event) {
	a.handle(event)
}

func TestSupervisor_Temporary(t *testing.T) {
	var runs atomic.Int32

	supervisor := sup.NewSupervisor("sup", sup.Temporary).
		AddActors(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
			runs.Add(1)
			panic("fatal error")
		}))

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if runs.Load() != 1 {
		t.Fatalf("expected 1 run, got %d", runs.Load())
	}
}

func TestSupervisor_Transient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		var attempts []time.Time

		supervisor := sup.NewSupervisor("sup", sup.Transient).
			SetRestartDelay(time.Second).
			AddActors(sup.ActorFunc(t.Name(), func(ctx context.Context) error {
				attempts = append(attempts, time.Now())
				count := runs.Add(1)
				if count == 1 {
					return errors.New("abnormal exit")
				}
				return nil
			}))

		if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		if runs.Load() != 2 {
			t.Fatalf("expected 2 runs, got %d", runs.Load())
		}
		if delay := attempts[1].Sub(attempts[0]); delay != time.Second {
			t.Fatalf("expected one-second restart delay, got %v", delay)
		}
	})
}

func TestSupervisor_PermanentRestartsCleanExit(t *testing.T) {
	var runs atomic.Int32
	supervisor := sup.NewSupervisor("sup", sup.Permanent).
		AddActors(sup.ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return nil
		})).
		SetRestartDelay(0).
		SetRestartLimit(2, time.Second)

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
	worker := sup.ActorFunc("worker", func(context.Context) error {
		runs.Add(1)
		return errors.New("fail")
	})
	observer := newEventHandlerActor("observer", func(event sup.Event) {
		if event.Type == sup.EventActorRestarting && event.Actor == worker {
			close(restarting)
		}
	})
	supervisor := sup.NewSupervisor("sup", sup.Transient).
		AddActors(worker, observer).
		SetRestartDelay(time.Hour)

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
	supervisor := sup.NewSupervisor("sup", sup.Permanent).
		AddActors(sup.ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return errors.New("fail")
		})).
		SetRestartDelay(0).
		SetRestartLimit(3, time.Second)

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

	supervisor := sup.NewSupervisor("sup", sup.Transient).
		AddActors(actor).
		SetRestartDelayFunc(func(restartCount int) time.Duration {
			mu.Lock()
			counts = append(counts, restartCount)
			mu.Unlock()
			return 0
		})

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
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		supervisor := sup.NewSupervisor("sup", sup.Transient).
			AddActors(sup.ActorFunc("worker", func(context.Context) error {
				run := runs.Add(1)
				if run == 3 {
					synctest.Sleep(50 * time.Millisecond)
				}
				if run < 5 {
					return errors.New("fail")
				}
				return nil
			})).
			SetRestartDelay(0).
			SetRestartLimit(2, 20*time.Millisecond)

		if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
			t.Fatalf("expected restart window to reset, got %v", err)
		}
		if runs.Load() != 5 {
			t.Fatalf("expected five runs, got %d", runs.Load())
		}
	})
}

func TestSupervisor_PanicStackTrace(t *testing.T) {
	var capturedErr error
	worker := sup.ActorFunc(t.Name(), func(ctx context.Context) error {
		panic("extreme failure")
	})
	observer := newEventHandlerActor("observer", func(event sup.Event) {
		if event.Type == sup.EventActorStopped && event.Actor == worker {
			capturedErr = event.Err
		}
	})

	supervisor := sup.NewSupervisor("sup", sup.Temporary).
		AddActors(worker, observer)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if capturedErr == nil || !strings.Contains(capturedErr.Error(), "stack") {
		t.Fatal("expected stack trace in panic recovery")
	}
}

func TestSupervisor_EventHandlerReceivesRichEvents(t *testing.T) {
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
	observer := newEventHandlerActor("observer", func(event sup.Event) {
		if event.Actor != actor {
			return
		}
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	supervisor := sup.NewSupervisor("root", sup.Transient).
		AddActors(actor, observer).
		SetRestartDelay(0)

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

func TestSupervisor_EventHandlerPanicDoesNotInterruptSupervision(t *testing.T) {
	var received atomic.Int32
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	panicking := newEventHandlerActor("panicking-observer", func(sup.Event) {
		panic("handler failure")
	})
	recording := newEventHandlerActor("recording-observer", func(event sup.Event) {
		if event.Actor == actor {
			received.Add(1)
		}
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(actor, panicking, recording)

	if err := runUntilSupervisorEmpty(t, supervisor); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if received.Load() != 3 {
		t.Fatalf("expected second event handler to receive all 3 events, got %d", received.Load())
	}
}

func TestSupervisorRejectsInvalidConfiguration(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })

	requirePanic(t, func() { sup.NewSupervisor("", sup.Transient) })
	requirePanic(t, func() { sup.NewSupervisor("root", sup.RestartPolicy(255)) })

	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).AddActors(emptyIDActor{})
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).AddActors(actor, actor)
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).AddActors(actor).AddActors(actor)
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).SetRestartDelay(-time.Nanosecond)
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).SetRestartLimit(0, time.Second)
	})
	requirePanic(t, func() {
		sup.NewSupervisor("root", sup.Transient).SetRestartLimit(1, 0)
	})
}

func TestSupervisor_ConfigurationMethodsAreChainable(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error { return nil })
	secondActor := sup.ActorFunc("second-worker", func(context.Context) error { return nil })

	supervisor := sup.NewSupervisor("root", sup.Transient).
		SetRestartDelay(0).
		SetRestartDelayFunc(func(int) time.Duration { return 0 }).
		SetRestartLimit(1, time.Second).
		AddActors(actor).
		AddActors(secondActor)

	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestSupervisor_EventHandlerPropagatesThroughSupervisorTree(t *testing.T) {
	var observedLeafWorkerStarted atomic.Int32

	worker := sup.ActorFunc("leaf-worker", func(ctx context.Context) error {
		return nil
	})

	leaf := sup.NewSupervisor("leaf", sup.Transient).AddActors(worker)

	middle := sup.NewSupervisor("middle", sup.Transient).AddActors(leaf)

	rootObserver := newEventHandlerActor("root-observer", func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor.ID() == "leaf-worker" {
			observedLeafWorkerStarted.Add(1)
		}
	})

	root := sup.NewSupervisor("root", sup.Transient).
		AddActors(middle, rootObserver)

	if err := runUntilSupervisorEmpty(t, root); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if observedLeafWorkerStarted.Load() != 1 {
		t.Fatalf("expected root handler to see leaf worker start, got %d", observedLeafWorkerStarted.Load())
	}
}

func TestSupervisor_LocalAndParentEventHandlersReceiveChildEvents(t *testing.T) {
	var parentObserved atomic.Int32
	var childObserved atomic.Int32

	parentObserver := newEventHandlerActor("parent-observer", func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor.ID() == "worker" {
			parentObserved.Add(1)
		}
	})
	childObserver := newEventHandlerActor("child-observer", func(event sup.Event) {
		if event.Type == sup.EventActorStarted && event.Actor.ID() == "worker" {
			childObserved.Add(1)
		}
	})

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		return nil
	})

	child := sup.NewSupervisor("child", sup.Transient).
		AddActors(worker, childObserver)

	root := sup.NewSupervisor("root", sup.Transient).
		AddActors(child, parentObserver)

	if err := runUntilSupervisorEmpty(t, root); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if parentObserved.Load() != 1 || childObserved.Load() != 1 {
		t.Fatalf("expected both handlers to receive one event, parent=%d child=%d",
			parentObserved.Load(), childObserved.Load())
	}
}

func runUntilSupervisorEmpty(t *testing.T, supervisor *sup.Supervisor) error {
	t.Helper()
	return supervisor.Run(t.Context())
}
