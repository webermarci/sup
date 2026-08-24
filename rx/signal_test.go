package rx_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/sup/rx"
)

func TestSignalValueReturnsInitialAndSetValue(t *testing.T) {
	signal := rx.NewSignal("count", 42)
	if got := signal.Value(); got != 42 {
		t.Fatalf("expected initial value 42, got %d", got)
	}

	signal.Set(10)
	if got := signal.Value(); got != 10 {
		t.Fatalf("expected set value 10, got %d", got)
	}
}

func TestSignalSubscribeStartsWithCurrentValue(t *testing.T) {
	signal := rx.NewSignal("count", 42)
	values := signal.Subscribe(t.Context())

	if got := receive(t, values); got != 42 {
		t.Fatalf("expected current value 42, got %d", got)
	}
}

func TestSignalSubscribersAreIndependent(t *testing.T) {
	signal := rx.NewSignal("count", 0)
	first := signal.Subscribe(t.Context())
	second := signal.Subscribe(t.Context())
	<-first
	<-second

	signal.Set(1)

	if got := receive(t, first); got != 1 {
		t.Fatalf("first subscriber: expected 1, got %d", got)
	}
	if got := receive(t, second); got != 1 {
		t.Fatalf("second subscriber: expected 1, got %d", got)
	}
}

func TestSignalSubscribeCoalescesToLatestValue(t *testing.T) {
	signal := rx.NewSignal("count", 0)
	values := signal.Subscribe(t.Context())

	signal.Set(1)
	signal.Set(2)
	signal.Set(3)

	if got := receive(t, values); got != 3 {
		t.Fatalf("expected latest value 3, got %d", got)
	}
}

func TestSignalSetPublishesRepeatedValue(t *testing.T) {
	signal := rx.NewSignal("count", 1)
	values := signal.Subscribe(t.Context())
	<-values

	signal.Set(1)

	if got := receive(t, values); got != 1 {
		t.Fatalf("expected repeated value 1, got %d", got)
	}
}

func TestSignalUpdateAtomicallyReplacesAndPublishesValue(t *testing.T) {
	signal := rx.NewSignal("count", 1)
	values := signal.Subscribe(t.Context())
	_ = receive(t, values)

	if got := signal.Update(func(current int) int { return current + 1 }); got != 2 {
		t.Fatalf("expected updated value 2, got %d", got)
	}
	if got := signal.Value(); got != 2 {
		t.Fatalf("expected stored value 2, got %d", got)
	}
	if got := receive(t, values); got != 2 {
		t.Fatalf("expected published value 2, got %d", got)
	}
}

func TestSignalUpdateSerializesConcurrentUpdates(t *testing.T) {
	signal := rx.NewSignal("count", 0)
	var updates sync.WaitGroup

	for range 100 {
		updates.Go(func() {
			for range 100 {
				signal.Update(func(current int) int { return current + 1 })
			}
		})
	}

	updates.Wait()
	if got := signal.Value(); got != 10_000 {
		t.Fatalf("expected 10000 updates, got %d", got)
	}
}

func TestSignalCanceledSubscriptionCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	signal := rx.NewSignal("count", 0)
	values := signal.Subscribe(ctx)
	<-values

	cancel()

	select {
	case _, ok := <-values:
		if ok {
			t.Fatal("expected subscription to close")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription to close")
	}
}

func TestSignalAlreadyCanceledSubscriptionIsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	values := rx.NewSignal("count", 0).Subscribe(ctx)
	if _, ok := <-values; ok {
		t.Fatal("expected an already-canceled subscription to be closed")
	}
}

func TestSignalWatchStartsAndCoalesces(t *testing.T) {
	signal := rx.NewSignal("count", 0)
	changes := signal.Watch(t.Context())

	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial change notification")
	}

	signal.Set(1)
	signal.Set(2)
	signal.Set(3)

	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coalesced change notification")
	}

	select {
	case <-changes:
		t.Fatal("expected pending notifications to be coalesced")
	default:
	}
}

func TestSignalRejectsEmptyID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected empty id to panic")
		}
	}()
	rx.NewSignal("", 0)
}

func receive[V any](t testing.TB, input <-chan V) V {
	t.Helper()
	select {
	case value, ok := <-input:
		if !ok {
			t.Fatal("channel closed before value")
		}
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for value")
		var zero V
		return zero
	}
}
