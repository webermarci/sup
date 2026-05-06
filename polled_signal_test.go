package sup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPolledSignal_DefaultValue(t *testing.T) {
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 42, nil
	})

	go signal.Run(t.Context())

	if v := signal.Read(); v != 0 {
		t.Errorf("expected zero value, got %d", v)
	}
}

func TestPolledSignal_InitialValue(t *testing.T) {
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 99, nil
	}).WithInitialValue(7)

	go signal.Run(t.Context())

	if v := signal.Read(); v != 7 {
		t.Errorf("expected initial value 7, got %d", v)
	}
}

func TestPolledSignal_Value(t *testing.T) {
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 42, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(t.Context())

	time.Sleep(50 * time.Millisecond)

	if v := signal.Read(); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestPolledSignal_ErrorSkipsUpdate(t *testing.T) {
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 0, errors.New("oops")
	}).
		WithInitialValue(5).
		WithInterval(10 * time.Millisecond)

	go signal.Run(t.Context())

	time.Sleep(50 * time.Millisecond)

	if v := signal.Read(); v != 5 {
		t.Errorf("expected value to stay at 5, got %d", v)
	}
}

func TestPolledSignal_Subscribe(t *testing.T) {
	ctx := t.Context()

	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 42, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for signal value")
	}
}

func TestPolledSignal_MultipleSubscribers(t *testing.T) {
	ctx := t.Context()

	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 55, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(ctx)

	ch1 := signal.Subscribe(ctx)
	ch2 := signal.Subscribe(ctx)

	for i, ch := range []<-chan int{ch1, ch2} {
		select {
		case v := <-ch:
			if v != 55 {
				t.Errorf("subscriber %d: expected 55, got %d", i+1, v)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out waiting for value", i+1)
		}
	}
}

func TestPolledSignal_UnsubscribeOnContextCancel(t *testing.T) {
	ctx := t.Context()

	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 1, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(ctx)

	subCtx, subCancel := context.WithCancel(ctx)
	ch := signal.Subscribe(subCtx)
	subCancel()

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel was not closed after context cancel")
		}
	}
}

func TestPolledSignal_InitialNotifyEnabled(t *testing.T) {
	ctx := t.Context()

	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 0, nil
	}).
		WithInitialValue(42).
		WithInitialNotify(true).
		WithInterval(time.Hour)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 42 {
			t.Fatalf("expected initial notify value 42, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestPolledSignal_InitialNotifyDisabled(t *testing.T) {
	ctx := t.Context()

	// Create a signal that will NEVER naturally poll during the test
	// because we set the interval to 1 Hour and return an error on the initial poll.
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 0, context.Canceled // Return an error so it skips the initial poll broadcast
	}).
		WithInitialValue(42).
		WithInterval(time.Hour)

	// Since InitialNotify is false (by default), subscribing should NOT
	// send the cached initial value of 42.
	ch := signal.Subscribe(ctx)

	// Start the actor
	go signal.Run(ctx)

	// We should receive absolutely nothing.
	select {
	case v := <-ch:
		t.Errorf("expected no initial notification, got %d", v)
	case <-time.After(100 * time.Millisecond):
		// correct, we successfully subscribed without getting spammed by the cached value!
	}
}

func TestPolledSignal_ActorInterface(t *testing.T) {
	signal := NewPolledSignal(t.Name(), func(_ context.Context) (int, error) {
		return 0, errors.New("fail")
	})

	if _, ok := any(signal).(Actor); !ok {
		t.Fatal("signal does not implement sup.Actor interface")
	}
}

func TestPolledSignal_WithEqual(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var pollCount int32
	var currentVal int32 = 42

	signal := NewPolledSignal("test-signal", func(ctx context.Context) (int, error) {
		atomic.AddInt32(&pollCount, 1)
		return int(atomic.LoadInt32(&currentVal)), nil
	}).
		WithInterval(10 * time.Millisecond).
		WithEqual(func(a, b int) bool { return a == b }).
		WithInitialNotify(false)

	go signal.Run(ctx)
	time.Sleep(20 * time.Millisecond) // Let it start

	ch := signal.Subscribe(ctx)

	// --- Scenario 1: Same value ---
	// Wait for several poll cycles (at least 50ms)
	time.Sleep(50 * time.Millisecond)

	// Ensure the poller is actually running
	if atomic.LoadInt32(&pollCount) < 3 {
		t.Fatal("Poller isn't running fast enough for the test")
	}

	// Verify nothing was broadcast because the value stayed 42
	select {
	case v := <-ch:
		t.Fatalf("Expected no broadcast for identical values, but got %d", v)
	default:
		// Success!
	}

	// --- Scenario 2: Value changes ---
	atomic.StoreInt32(&currentVal, 99)

	// It should now detect the difference and broadcast
	select {
	case v := <-ch:
		if v != 99 {
			t.Errorf("Expected updated value 99, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for new value after mutation")
	}
}
