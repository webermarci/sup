package bus

import (
	"context"
	"testing"
	"time"
)

func TestThrottle_InitialValue(t *testing.T) {
	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(42)

	throttle := NewThrottle("throttle", trigger, 100*time.Millisecond)

	// Before any running or updates, it should immediately read the source's initial value
	if got := throttle.Read(); got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}
}

func TestThrottle_FiresImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(0)
	go trigger.Run(ctx)

	// Throttle to 1 second
	throttle := NewThrottle("throttle", trigger, time.Second).WithInitialNotify(false)
	go throttle.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions
	ch := throttle.Subscribe(ctx)

	start := time.Now()
	trigger.Write(ctx, 1)

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

func TestThrottle_TrailingEdge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(0)
	go trigger.Run(ctx)

	throttle := NewThrottle("throttle", trigger, 200*time.Millisecond).WithInitialNotify(false)
	go throttle.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions
	ch := throttle.Subscribe(ctx)

	// Fire one to close the window
	trigger.Write(ctx, 1)
	<-ch // drain the immediate first fire

	// Fire three times rapidly while the window is closed
	trigger.Write(ctx, 2)
	time.Sleep(10 * time.Millisecond)
	trigger.Write(ctx, 3)
	time.Sleep(10 * time.Millisecond)
	trigger.Write(ctx, 4)

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

func TestThrottle_CloseCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error { return nil })
	go trigger.Run(ctx)

	throttle := NewThrottle("throttle", trigger, 100*time.Millisecond)
	go throttle.Run(ctx)

	time.Sleep(20 * time.Millisecond)
	ch := throttle.Subscribe(ctx)

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
