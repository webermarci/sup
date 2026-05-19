package sup

import (
	"context"
	"testing"
	"time"
)

func TestThrottledSignal_InitialValue(t *testing.T) {
	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	pushed.SetInitialValue(42)

	throttled := NewThrottledSignal("throttle", pushed, 100*time.Millisecond)

	// Before any running or updates, it should immediately read the source's initial value
	if got := throttled.Read(); got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}
}

func TestThrottledSignal_FiresImmediately(t *testing.T) {
	ctx := t.Context()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	// Throttle to 1 second
	throttled := NewThrottledSignal("throttle", pushed, time.Second)

	go throttled.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions
	ch := throttled.Subscribe(ctx)

	start := time.Now()
	pushed.Write(ctx, 1)

	// Because the throttle window is completely open, the first value
	// should arrive instantly, well before the 1s interval.
	select {
	case v := <-ch:
		if v != 1 {
			t.Errorf("Expected 1, got %d", v)
		}
		if time.Since(start) > 50*time.Millisecond {
			t.Errorf("Throttle delayed the first event artificially")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Throttle completely blocked the first event")
	}
}

func TestThrottledSignal_TrailingEdge(t *testing.T) {
	ctx := t.Context()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	throttled := NewThrottledSignal("throttle", pushed, 200*time.Millisecond)
	go throttled.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions
	ch := throttled.Subscribe(ctx)

	// Fire one to close the window
	pushed.Write(ctx, 1)
	<-ch // drain the immediate first fire

	// Fire three times rapidly while the window is closed
	pushed.Write(ctx, 2)
	time.Sleep(10 * time.Millisecond)
	pushed.Write(ctx, 3)
	time.Sleep(10 * time.Millisecond)
	pushed.Write(ctx, 4)

	start := time.Now()

	// Wait for the 200ms throttle timer to elapse.
	// It should ONLY emit the last value (4).
	select {
	case v := <-ch:
		if v != 4 {
			t.Errorf("Expected the trailing edge value 4, got %d", v)
		}
		if time.Since(start) < 150*time.Millisecond {
			t.Errorf("Throttle fired too early")
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timed out waiting for throttled value")
	}

	// Wait another interval to ensure it doesn't emit duplicate trailing values
	select {
	case v := <-ch:
		t.Fatalf("Received duplicate value %d from empty interval", v)
	case <-time.After(250 * time.Millisecond):
		// Expected: silent because nothing happened
	}
}

func TestThrottledSignal_CloseCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	throttled := NewThrottledSignal("throttle", pushed, 100*time.Millisecond)
	go throttled.Run(ctx)

	time.Sleep(20 * time.Millisecond)
	ch := throttled.Subscribe(ctx)

	// Cancel the context to shut down the actor
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("Expected channel to be closed, but received a value")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Channel was not closed during context cancellation")
	}
}

func TestThrottledSignal_Inspect(t *testing.T) {
	src := NewPushedSignal("src2", func(ctx context.Context, v int) error { return nil })
	th := NewThrottledSignal("th", src, 2*time.Second)
	spec := th.Inspect()

	if spec.Kind != "throttled_signal" {
		t.Fatalf("expected kind throttled_signal, got %q", spec.Kind)
	}

	if len(spec.Dependencies) != 1 || spec.Dependencies[0] != src.ID() {
		t.Fatalf("expected dependency %q, got %v", src.ID(), spec.Dependencies)
	}

	if spec.Metadata["interval"] != (2 * time.Second).String() {
		t.Fatalf("expected interval=%q, got %q", (2 * time.Second).String(), spec.Metadata["interval"])
	}
}
