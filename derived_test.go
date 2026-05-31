package sup

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

	signal := NewSignal("pushed", 0)
	go signal.Run(ctx)

	var count atomic.Int32
	computed := NewDerived("computed", func() int32 {
		return count.Add(1)
	}, signal)

	go computed.Run(ctx)

	// Wait for subscriptions to be established.
	// In a real system, we'd wait for a ready signal, but here we just wait briefly.
	time.Sleep(20 * time.Millisecond)

	if got := computed.Read(); got != 1 {
		t.Errorf("Expected initial value 1, got %d", got)
	}

	signal.Write(ctx, 100)

	// Wait for update
	waitForValue(t, computed, 2)
}

func TestDerived_Subscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := NewSignal("pushed", 0)
	go signal.Run(ctx)

	val := int32(10)
	computed := NewDerived("computed", func() int32 {
		return atomic.LoadInt32(&val)
	}, signal).InitialNotify()

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
	signal.Write(ctx, 1)

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

	signal := NewSignal("pushed", 0)
	go signal.Run(ctx)

	computed := NewDerived("computed", func() int {
		return 0
	}, signal).InitialNotify()

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
	signal.Write(ctx, 1)

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

	t1 := NewSignal("t1", 0)
	t2 := NewSignal("t2", 0)
	go t1.Run(ctx)
	go t2.Run(ctx)

	var count atomic.Int32
	computed := NewDerived("computed", func() int32 {
		return count.Add(1)
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

func waitForValue[V comparable](t *testing.T, r ReaderSignal[V], want V) {
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

func TestDerived_GlitchFreeDiamond(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	root := NewSignal("root", 0)
	go root.Run(ctx)

	var valB, valC int32
	nodeB := NewDerived("B", func() int32 {
		atomic.AddInt32(&valB, 1)
		return atomic.LoadInt32(&valB)
	}, root)

	nodeC := NewDerived("C", func() int32 {
		atomic.AddInt32(&valC, 1)
		return atomic.LoadInt32(&valC)
	}, root)

	go nodeB.Run(ctx)
	go nodeC.Run(ctx)

	var evalCount atomic.Int32
	nodeD := NewDerived("D", func() int32 {
		evalCount.Add(1)
		b := nodeB.Read()
		c := nodeC.Read()
		return b + c
	}, nodeB, nodeC).BatchWindow(20 * time.Millisecond)

	go nodeD.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	evalCount.Store(0)
	root.Write(ctx, 1)
	waitForValue(t, nodeD, 4)

	count := evalCount.Load()
	if count != 1 {
		t.Errorf("Glitch detected! Expected node D to evaluate exactly 1 time, but it evaluated %d times", count)
	}
}

func TestDerived_Inspect(t *testing.T) {
	p := NewSignal("p1", 0)
	q := NewSignal("p2", 0)
	c := NewDerived("comp", func() int { return 42 }, p, q)

	spec := c.Inspect()

	if spec.Kind != "derived" {
		t.Fatalf("expected kind derived, got %q", spec.Kind)
	}

	if len(spec.Dependencies) != 2 || spec.Dependencies[0] != p.ID() || spec.Dependencies[1] != q.ID() {
		t.Fatalf("expected dependencies [%q %q], got %v", p.ID(), q.ID(), spec.Dependencies)
	}

	if spec.Metadata["batch_window"] != (5 * time.Millisecond).String() {
		t.Fatalf("expected batch_window=%q, got %q", (5 * time.Millisecond).String(), spec.Metadata["batch_window"])
	}
}

func TestDerived_EqualSuppressesUnchangedNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	dependency := NewSignal("dependency", 0)
	var value atomic.Int32
	value.Store(10)

	derived := NewDerived("derived", func() int32 {
		return value.Load()
	}, dependency).
		Equal(func(a, b int32) bool { return a == b }).
		BatchWindow(time.Millisecond)

	updates := derived.Subscribe(ctx)
	done := make(chan error, 1)
	go func() {
		done <- derived.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	if err := dependency.Write(ctx, 1); err != nil {
		t.Fatalf("dependency write failed: %v", err)
	}

	select {
	case got := <-updates:
		t.Fatalf("expected equal value to suppress notification, got %d", got)
	case err := <-done:
		t.Fatalf("derived exited early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	value.Store(20)
	if err := dependency.Write(ctx, 2); err != nil {
		t.Fatalf("dependency write failed: %v", err)
	}

	select {
	case got := <-updates:
		if got != 20 {
			t.Fatalf("expected updated derived value 20, got %d", got)
		}
	case err := <-done:
		t.Fatalf("derived exited early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for changed derived value")
	}
}

func TestDerivedEffectMethod(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	dependency := NewSignal("dependency", 0)
	var value atomic.Int32
	value.Store(1)

	derived := NewDerived("derived", func() int32 {
		return value.Load()
	}, dependency).
		InitialNotify().
		BatchWindow(time.Millisecond)

	effectValues := make(chan int32, 2)
	effect := derived.Effect("effect", func(ctx context.Context, value int32) error {
		effectValues <- value
		return nil
	})

	derivedDone := make(chan error, 1)
	go func() {
		derivedDone <- derived.Run(ctx)
	}()

	effectDone := make(chan error, 1)
	go func() {
		effectDone <- effect.Run(ctx)
	}()

	select {
	case got := <-effectValues:
		if got != 1 {
			t.Fatalf("expected initial effect value 1, got %d", got)
		}
	case err := <-derivedDone:
		t.Fatalf("derived exited early: %v", err)
	case err := <-effectDone:
		t.Fatalf("effect exited early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial derived effect")
	}

	// Give Derived.Run time to subscribe to the dependency before triggering it.
	time.Sleep(20 * time.Millisecond)

	value.Store(2)
	if err := dependency.Write(ctx, 1); err != nil {
		t.Fatalf("dependency write failed: %v", err)
	}

	select {
	case got := <-effectValues:
		if got != 2 {
			t.Fatalf("expected updated effect value 2, got %d", got)
		}
	case err := <-derivedDone:
		t.Fatalf("derived exited early: %v", err)
	case err := <-effectDone:
		t.Fatalf("effect exited early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for updated derived effect")
	}
}
