package rx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/webermarci/sup/rx"
)

func TestWaitForAcceptsCurrentValue(t *testing.T) {
	ready := rx.NewSignal("ready", true)

	value, err := rx.WaitFor(t.Context(), ready, func(value bool) bool { return value })
	if err != nil {
		t.Fatal(err)
	}
	if !value {
		t.Fatal("expected current value to be accepted")
	}
}

func TestWaitForAcceptsLaterValue(t *testing.T) {
	status := rx.NewSignal("status", "starting")
	result := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		value, err := rx.WaitFor(t.Context(), status, func(value string) bool {
			return value == "ready"
		})
		result <- value
		done <- err
	}()

	status.Set("running")
	status.Set("ready")

	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, result); got != "ready" {
		t.Fatalf("expected ready, got %q", got)
	}
}

func TestWaitForReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	value, err := rx.WaitFor(ctx, rx.NewSignal("ready", false), func(value bool) bool {
		return value
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if value {
		t.Fatal("expected zero value after cancellation")
	}
}

func TestWaitForReturnsErrorWhenReadableCloses(t *testing.T) {
	values := make(chan int)
	close(values)
	readable := &stubReadable[int]{subscribe: func(context.Context) <-chan int {
		return values
	}}

	value, err := rx.WaitFor(t.Context(), readable, func(int) bool { return false })
	if !errors.Is(err, rx.ErrReadableClosed) {
		t.Fatalf("expected ErrReadableClosed, got %v", err)
	}
	if value != 0 {
		t.Fatalf("expected zero value, got %d", value)
	}
}

func TestWaitForCancelsPrivateSubscription(t *testing.T) {
	subscription := make(chan context.Context, 1)
	values := make(chan int, 1)
	values <- 42
	readable := &stubReadable[int]{subscribe: func(ctx context.Context) <-chan int {
		subscription <- ctx
		return values
	}}

	value, err := rx.WaitFor(t.Context(), readable, func(value int) bool { return value == 42 })
	if err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("expected 42, got %d", value)
	}

	ctx := receive(t, subscription)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("subscription context was not canceled")
	}
}

type stubReadable[V any] struct {
	subscribe func(context.Context) <-chan V
}

func (*stubReadable[V]) ID() string { return "stub" }

func (*stubReadable[V]) Value() (zero V) { return zero }

func (s *stubReadable[V]) Subscribe(ctx context.Context) <-chan V {
	return s.subscribe(ctx)
}

func (*stubReadable[V]) Watch(ctx context.Context) <-chan struct{} {
	changes := make(chan struct{})
	context.AfterFunc(ctx, func() { close(changes) })
	return changes
}
