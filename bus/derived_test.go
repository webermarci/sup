package bus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDerived_InitialValue(t *testing.T) {
	d := NewDerived("test", func() int {
		return 42
	})

	if got := d.Read(); got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}
}

func TestDerived_UpdateOnDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	})
	go trigger.Run(ctx)

	var count int32
	derived := NewDerived("derived", func() int32 {
		return atomic.AddInt32(&count, 1)
	}, trigger)

	go derived.Run(ctx)

	// Wait for subscriptions to be established.
	// In a real system, we'd wait for a ready signal, but here we just wait briefly.
	time.Sleep(20 * time.Millisecond)

	// Initial value should be 1 (from NewDerived)
	if got := derived.Read(); got != 1 {
		t.Errorf("Expected initial value 1, got %d", got)
	}

	// Trigger an update
	trigger.Write(ctx, 100)

	// Wait for update
	waitForValue(t, derived, 2)
}

func TestDerived_Subscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	})
	go trigger.Run(ctx)

	val := int32(10)
	derived := NewDerived("derived", func() int32 {
		return atomic.LoadInt32(&val)
	}, trigger)

	go derived.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	ch := derived.Subscribe(ctx)

	// First value should be initial value
	select {
	case v := <-ch:
		if v != 10 {
			t.Errorf("Expected initial value 10, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for initial value")
	}

	// Update dependency
	atomic.StoreInt32(&val, 20)
	trigger.Write(ctx, 1)

	// Should receive update
	select {
	case v := <-ch:
		if v != 20 {
			t.Errorf("Expected updated value 20, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timed out waiting for updated value")
	}
}

func TestDerived_Notify(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	trigger := NewTrigger("trigger", func(ctx context.Context, v int) error {
		return nil
	})
	go trigger.Run(ctx)

	derived := NewDerived("derived", func() int {
		return 0
	}, trigger)

	go derived.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	ch := derived.Watch(ctx)

	// The first notification is sent immediately upon subscription because notify=true in SubscribeNotifications
	select {
	case <-ch:
		// initial notification
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for initial notification")
	}

	// Update dependency
	trigger.Write(ctx, 1)

	// Should receive notification
	select {
	case <-ch:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timed out waiting for notification")
	}
}

func TestDerived_MultipleDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	t1 := NewTrigger("t1", func(ctx context.Context, v int) error { return nil })
	t2 := NewTrigger("t2", func(ctx context.Context, v int) error { return nil })
	go t1.Run(ctx)
	go t2.Run(ctx)

	var count int32
	derived := NewDerived("derived", func() int32 {
		return atomic.AddInt32(&count, 1)
	}, t1, t2)

	go derived.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Initial value
	if got := derived.Read(); got != 1 {
		t.Errorf("Expected 1, got %d", got)
	}

	// Trigger t1
	t1.Write(ctx, 1)
	waitForValue(t, derived, 2)

	// Trigger t2
	t2.Write(ctx, 1)
	waitForValue(t, derived, 3)
}

func waitForValue[V comparable](t *testing.T, r Reader[V], want V) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		if r.Read() == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("Timed out waiting for value %v, got %v", want, r.Read())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
