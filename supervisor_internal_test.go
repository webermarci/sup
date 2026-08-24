package sup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisor_EmptyRunReturns(t *testing.T) {
	if err := NewSupervisor("sup", Transient).Run(t.Context()); err != nil {
		t.Fatalf("empty Run returned error: %v", err)
	}
}

func TestSupervisor_SequentialRuns(t *testing.T) {
	var runs atomic.Int32
	started := make(chan struct{}, 2)
	supervisor := NewSupervisor("sup", Temporary).
		AddActors(ActorFunc("worker", func(ctx context.Context) error {
			runs.Add(1)
			started <- struct{}{}
			<-ctx.Done()
			return nil
		}))

	for run := range 2 {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- supervisor.Run(ctx) }()
		<-started
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("run %d returned error: %v", run+1, err)
		}
	}
	if runs.Load() != 2 {
		t.Fatalf("expected two actor runs, got %d", runs.Load())
	}
}

func TestSupervisor_CancelsAttemptContextBeforeRestart(t *testing.T) {
	var runs atomic.Int32
	firstCanceled := make(chan struct{})
	secondFinished := make(chan struct{})
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
			close(secondFinished)
			return nil
		case <-time.After(time.Second):
			return errors.New("previous attempt context was not canceled")
		}
	})

	supervisor := NewSupervisor("sup", Transient).
		AddActors(actor).
		SetRestartDelay(0)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()
	<-secondFinished
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestSupervisor_CancellationNormalizesChildError(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		supervisor := NewSupervisor("sup", Temporary).
			AddActors(ActorFunc("worker", func(context.Context) error {
				<-release
				return errors.New("failure during shutdown")
			}))

		done := make(chan error, 1)
		go func() { done <- supervisor.Run(ctx) }()
		cancel()
		close(release)

		if err := <-done; err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	}
}
