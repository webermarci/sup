package sup

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisor_EmptyRunReturns(t *testing.T) {
	if err := NewSupervisor("sup").Run(t.Context()); err != nil {
		t.Fatalf("empty Run returned error: %v", err)
	}
}

func TestSupervisor_RejectsConcurrentRun(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	supervisor := NewSupervisor("sup", WithActors(ActorFunc("worker", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	})))

	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()
	<-started

	if err := supervisor.Run(t.Context()); !errors.Is(err, ErrSupervisorRunning) {
		t.Fatalf("expected ErrSupervisorRunning, got %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestSupervisor_SequentialRuns(t *testing.T) {
	var runs atomic.Int32
	supervisor := NewSupervisor("sup",
		WithActors(ActorFunc("worker", func(context.Context) error {
			runs.Add(1)
			return nil
		})),
		WithPolicy(Temporary),
	)

	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("expected two actor runs, got %d", runs.Load())
	}
}

func TestSupervisor_CancelsAttemptContextBeforeRestart(t *testing.T) {
	var runs atomic.Int32
	firstCanceled := make(chan struct{})
	actor := ActorFunc("worker", func(ctx context.Context) error {
		if runs.Add(1) == 1 {
			go func() {
				<-ctx.Done()
				close(firstCanceled)
			}()
			return errors.New("restart")
		}

		select {
		case <-firstCanceled:
			return nil
		case <-time.After(time.Second):
			return errors.New("previous attempt context was not canceled")
		}
	})

	supervisor := NewSupervisor("sup",
		WithActors(actor),
		WithPolicy(Transient),
		WithRestartDelay(0),
	)
	if err := supervisor.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestSupervisor_CancellationNormalizesChildError(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		supervisor := NewSupervisor("sup",
			WithActors(ActorFunc("worker", func(context.Context) error {
				<-release
				return errors.New("failure during shutdown")
			})),
			WithPolicy(Temporary),
		)

		done := make(chan error, 1)
		go func() { done <- supervisor.Run(ctx) }()
		waitForSupervisorRunning(t, supervisor)
		cancel()
		close(release)

		if err := <-done; err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	}
}

func waitForSupervisorRunning(t *testing.T, supervisor *Supervisor) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if supervisor.running.Load() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for supervisor to start")
		}
		runtime.Gosched()
	}
}
