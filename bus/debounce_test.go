package bus

import (
	"context"
	"testing"
	"time"
)

func TestDebounce_InitialValue(t *testing.T) {
	// Setup a trigger with an initial value
	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(42)

	debounce := NewDebounce("debounce", trigger, 100*time.Millisecond)

	// Before any running or updates, it should immediately read the source's initial value
	if got := debounce.Read(); got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}
}

func TestDebounce_Behavior(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(0)
	go trigger.Run(ctx)

	// Set wait to 100ms. Disable initial notify so we only test reactive updates.
	debounce := NewDebounce("debounce", trigger, 100*time.Millisecond).WithInitialNotify(false)
	go debounce.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions to establish
	ch := debounce.Subscribe(ctx)

	// Fire 3 rapid updates spaced by 30ms (which is less than the 100ms debounce wait)
	trigger.Write(ctx, 1)
	time.Sleep(30 * time.Millisecond)
	trigger.Write(ctx, 2)
	time.Sleep(30 * time.Millisecond)
	trigger.Write(ctx, 3)

	// We should not have received anything yet, because the timer keeps resetting
	select {
	case v := <-ch:
		t.Fatalf("Received value %d too early, debounce failed", v)
	case <-time.After(20 * time.Millisecond):
		// Expected: still waiting for the silence period
	}

	// Now wait for the 100ms silence period to elapse after the last write
	select {
	case v := <-ch:
		if v != 3 {
			t.Errorf("Expected to receive the latest value 3, got %d", v)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("Timed out waiting for debounced value")
	}
}

func TestDebounce_MaxWait(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	}).WithInitialValue(0)
	go trigger.Run(ctx)

	// Set wait to 200ms, but force a publish if 300ms passes
	debounce := NewDebounce("debounce", trigger, 200*time.Millisecond).
		WithMaxWait(300 * time.Millisecond).
		WithInitialNotify(false)
	go debounce.Run(ctx)

	time.Sleep(20 * time.Millisecond) // Wait for subscriptions
	ch := debounce.Subscribe(ctx)

	start := time.Now()

	// Create an infinite stream of spam that never rests for 200ms
	go func() {
		for i := 1; i <= 10; i++ {
			trigger.Write(ctx, i)
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Without MaxWait, a 200ms debounce would NEVER fire here because events
	// keep arriving every 50ms. With MaxWait(300ms), it should force a publish.
	select {
	case v := <-ch:
		elapsed := time.Since(start)

		// It should have waited at least the MaxWait time (approx 300ms)
		if elapsed < 250*time.Millisecond {
			t.Errorf("Fired too early: %v", elapsed)
		}

		// It should be an intermediate value (roughly the 5th or 6th write),
		// proving it fired BEFORE the spam stopped.
		if v == 10 {
			t.Errorf("Should have forced publish during the burst, but got the final value %d", v)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("MaxWait did not force a publish, stream was starved")
	}
}

func TestDebounce_CloseCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error { return nil })
	go trigger.Run(ctx)

	debounce := NewDebounce("debounce", trigger, 100*time.Millisecond)
	go debounce.Run(ctx)

	time.Sleep(20 * time.Millisecond)
	ch := debounce.Subscribe(ctx)

	// Cancel the context, which should shut down the Run loop and close all subscriber channels
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
