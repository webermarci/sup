package sup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestComputedSignal_InitialValue(t *testing.T) {
	d := NewComputedSignal("test", func() int {
		return 42
	})

	if got := d.Read(); got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}
}

func TestComputedSignal_UpdateOnDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	var count int32
	computed := NewComputedSignal("computed", func() int32 {
		return atomic.AddInt32(&count, 1)
	}, pushed)

	go computed.Run(ctx)

	// Wait for subscriptions to be established.
	// In a real system, we'd wait for a ready signal, but here we just wait briefly.
	time.Sleep(20 * time.Millisecond)

	if got := computed.Read(); got != 1 {
		t.Errorf("Expected initial value 1, got %d", got)
	}

	pushed.Write(ctx, 100)

	// Wait for update
	waitForValue(t, computed, 2)
}

func TestComputedSignal_Subscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	val := int32(10)
	computed := NewComputedSignal("computed", func() int32 {
		return atomic.LoadInt32(&val)
	}, pushed).WithInitialNotify(true)

	go computed.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	ch := computed.Subscribe(ctx)

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
	pushed.Write(ctx, 1)

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

func TestComputedSignal_Notify(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error {
		return nil
	})
	go pushed.Run(ctx)

	computed := NewComputedSignal("computed", func() int {
		return 0
	}, pushed).WithInitialNotify(true)

	go computed.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	ch := computed.Watch(ctx)

	// The first notification is sent immediately upon subscription because notify=true in SubscribeNotifications
	select {
	case <-ch:
		// initial notification
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for initial notification")
	}

	// Update dependency
	pushed.Write(ctx, 1)

	// Should receive notification
	select {
	case <-ch:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timed out waiting for notification")
	}
}

func TestComputedSignal_MultipleDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	t1 := NewPushedSignal("t1", func(ctx context.Context, v int) error { return nil })
	t2 := NewPushedSignal("t2", func(ctx context.Context, v int) error { return nil })
	go t1.Run(ctx)
	go t2.Run(ctx)

	var count int32
	computed := NewComputedSignal("computed", func() int32 {
		return atomic.AddInt32(&count, 1)
	}, t1, t2)

	go computed.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	// Initial value
	if got := computed.Read(); got != 1 {
		t.Errorf("Expected 1, got %d", got)
	}

	t1.Write(ctx, 1)
	waitForValue(t, computed, 2)

	t2.Write(ctx, 1)
	waitForValue(t, computed, 3)
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

func TestComputedSignal_GlitchFreeDiamond(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	root := NewPushedSignal("root", func(ctx context.Context, v int) error { return nil })
	go root.Run(ctx)

	var valB, valC int32
	nodeB := NewComputedSignal("B", func() int32 {
		atomic.AddInt32(&valB, 1)
		return atomic.LoadInt32(&valB)
	}, root)

	nodeC := NewComputedSignal("C", func() int32 {
		atomic.AddInt32(&valC, 1)
		return atomic.LoadInt32(&valC)
	}, root)

	go nodeB.Run(ctx)
	go nodeC.Run(ctx)

	var evalCount int32
	nodeD := NewComputedSignal("D", func() int32 {
		atomic.AddInt32(&evalCount, 1)
		b := nodeB.Read()
		c := nodeC.Read()
		return b + c
	}, nodeB, nodeC)

	nodeD.WithCoalesceWindow(20 * time.Millisecond)
	go nodeD.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	atomic.StoreInt32(&evalCount, 0)
	root.Write(ctx, 1)
	waitForValue(t, nodeD, 4)

	count := atomic.LoadInt32(&evalCount)
	if count != 1 {
		t.Errorf("Glitch detected! Expected node D to evaluate exactly 1 time, but it evaluated %d times", count)
	}
}

func TestComputedSignal_WithEqual(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pushed := NewPushedSignal("pushed", func(ctx context.Context, v int) error { return nil })
	go pushed.Run(ctx)

	var currentVal int32 = 10

	// Create a computed with a basic equality check
	computed := NewComputedSignal("computed", func() int {
		return int(atomic.LoadInt32(&currentVal))
	}, pushed).
		WithEqual(func(a, b int) bool { return a == b }).
		WithInitialNotify(false)

	go computed.Run(ctx)
	time.Sleep(20 * time.Millisecond)

	ch := computed.Subscribe(ctx)

	pushed.Write(ctx, 1)

	// It should evaluate, but it MUST NOT broadcast
	select {
	case <-ch:
		t.Fatal("Expected no broadcast because the value evaluated to equal")
	case <-time.After(100 * time.Millisecond):
		// Success! The broadcast was suppressed.
	}

	atomic.StoreInt32(&currentVal, 20)
	pushed.Write(ctx, 2)

	// It MUST broadcast the new value
	select {
	case v := <-ch:
		if v != 20 {
			t.Errorf("Expected updated value 20, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for updated value to broadcast")
	}
}
