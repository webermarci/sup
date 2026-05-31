package sup_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestEffectRunsWhenSignalUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	values := make(chan int, 2)

	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		values <- value
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	// Drain initial value from InitialNotify.
	select {
	case got := <-values:
		if got != 0 {
			t.Fatalf("expected initial value 0, got %d", got)
		}
	case err := <-done:
		t.Fatalf("effect exited early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial effect")
	}

	if err := signal.Write(ctx, 42); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 42 {
			t.Fatalf("expected effect value 42, got %d", got)
		}

	case err := <-done:
		t.Fatalf("effect exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean effect shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect shutdown")
	}
}

func TestEffectReturnsError(t *testing.T) {
	ctx := t.Context()

	expectedErr := errors.New("boom")

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		return expectedErr
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected effect error %v, got %v", expectedErr, err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect error")
	}
}

func TestEffectInitialNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 42).
		InitialNotify()

	values := make(chan int, 1)

	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		values <- value
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	select {
	case got := <-values:
		if got != 42 {
			t.Fatalf("expected initial value 42, got %d", got)
		}

	case err := <-done:
		t.Fatalf("effect exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial effect")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean effect shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect shutdown")
	}
}

func TestEffectDoesNotRunInitiallyWithoutInitialNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 42)

	var calls atomic.Int32

	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		calls.Add(1)
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("effect exited early: %v", err)

	case <-time.After(50 * time.Millisecond):
	}

	if calls.Load() != 0 {
		t.Fatalf("expected no initial effect call, got %d", calls.Load())
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean effect shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect shutdown")
	}
}

func TestSignalEffectMethod(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	values := make(chan int, 2)

	effect := signal.Effect("effect", func(ctx context.Context, value int) error {
		values <- value
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- effect.Run(ctx)
	}()

	select {
	case got := <-values:
		if got != 0 {
			t.Fatalf("expected initial effect value 0, got %d", got)
		}

	case err := <-done:
		t.Fatalf("effect exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial effect")
	}

	if err := signal.Write(ctx, 7); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 7 {
			t.Fatalf("expected effect value 7, got %d", got)
		}

	case err := <-done:
		t.Fatalf("effect exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean effect shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for effect shutdown")
	}
}

func TestEffectUnderSupervisor(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	values := make(chan int, 2)

	effect := signal.Effect("effect", func(ctx context.Context, value int) error {
		values <- value
		return nil
	})

	supervisor := sup.NewSupervisor("sup").
		Policy(sup.Transient).
		Actors(signal, effect)

	done := make(chan error, 1)
	go func() {
		done <- supervisor.Run(ctx)
	}()

	select {
	case got := <-values:
		if got != 0 {
			t.Fatalf("expected initial effect value 0, got %d", got)
		}

	case err := <-done:
		t.Fatalf("supervisor exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial supervised effect")
	}

	if err := signal.Write(ctx, 100); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 100 {
			t.Fatalf("expected effect value 100, got %d", got)
		}

	case err := <-done:
		t.Fatalf("supervisor exited early: %v", err)

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for supervised effect")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("expected clean/canceled supervisor shutdown, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for supervisor shutdown")
	}
}

func TestEffectInspect(t *testing.T) {
	signal := sup.NewSignal("count", 0)
	effect := sup.NewEffect("effect", signal, func(ctx context.Context, value int) error {
		return nil
	})

	spec := effect.Inspect()

	if spec.Kind != "effect" {
		t.Fatalf("expected kind effect, got %q", spec.Kind)
	}

	if len(spec.Dependencies) != 1 || spec.Dependencies[0] != "count" {
		t.Fatalf("expected dependency count, got %v", spec.Dependencies)
	}
}

func TestEffectRestartsUnderSupervisor(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		InitialNotify()

	var calls atomic.Int32

	effect := signal.Effect("effect", func(ctx context.Context, value int) error {
		count := calls.Add(1)
		if count == 1 {
			return errors.New("effect failed")
		}
		return nil
	})

	supervisor := sup.NewSupervisor("sup").
		Policy(sup.Transient).
		RestartDelay(time.Millisecond).
		Actors(signal, effect)

	done := make(chan error, 1)
	go func() {
		done <- supervisor.Run(ctx)
	}()

	deadline := time.After(time.Second)
	for {
		if calls.Load() >= 2 {
			cancel()

			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("expected clean/canceled supervisor shutdown, got %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for supervisor shutdown")
			}

			return
		}

		select {
		case err := <-done:
			t.Fatalf("supervisor exited early: %v", err)

		case <-deadline:
			t.Fatalf("expected restarted effect to be called, got %d calls", calls.Load())

		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
