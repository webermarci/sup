package sup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSignal_ReadReturnsInitialValue(t *testing.T) {
	signal := sup.NewSignal("count", 42)

	if got := signal.Read(); got != 42 {
		t.Fatalf("expected initial value 42, got %d", got)
	}
}

func TestSignal_WriteUpdatesValue(t *testing.T) {
	signal := sup.NewSignal("count", 0)

	if err := signal.Write(t.Context(), 10); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := signal.Read(); got != 10 {
		t.Fatalf("expected value 10, got %d", got)
	}
}

func TestSignal_WriteNotifiesSubscriber(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("count", 0)
	values := signal.Subscribe(ctx)

	if err := signal.Write(ctx, 10); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 10 {
			t.Fatalf("expected notification value 10, got %d", got)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber notification")
	}
}

func TestSignal_WriteDoesNotNotifyWhenEqual(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("count", 0).
		Equal(func(a, b int) bool {
			return a == b
		})

	values := signal.Subscribe(ctx)

	if err := signal.Write(ctx, 0); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case got := <-values:
		t.Fatalf("expected no notification, got %d", got)

	case <-time.After(50 * time.Millisecond):
	}
}

func TestSignal_InitialNotifySendsCurrentValue(t *testing.T) {
	signal := sup.NewSignal("count", 42).
		InitialNotify()

	values := signal.Subscribe(t.Context())

	select {
	case got := <-values:
		if got != 42 {
			t.Fatalf("expected initial notification value 42, got %d", got)
		}

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial notification")
	}
}

func TestSignal_BufferDropsWhenSubscriberBufferFull(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("count", 0).
		Buffer(1)
	values := signal.Subscribe(ctx)

	if err := signal.Write(ctx, 1); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := signal.Write(ctx, 2); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	select {
	case got := <-values:
		if got != 1 {
			t.Fatalf("expected buffered value 1, got %d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for buffered value")
	}

	select {
	case got := <-values:
		t.Fatalf("expected second notification to be dropped, got %d", got)
	case <-time.After(50 * time.Millisecond):
	}

	if got := signal.Read(); got != 2 {
		t.Fatalf("expected stored value to still update to 2, got %d", got)
	}
}

func TestSignal_WatchNotifiesOnWrite(t *testing.T) {
	ctx := t.Context()

	signal := sup.NewSignal("count", 0)
	updates := signal.Watch(ctx)

	if err := signal.Write(ctx, 1); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watch notification")
	}
}

func TestSignal_Inspect(t *testing.T) {
	signal := sup.NewSignal("count", 0).
		InitialNotify().
		Poll(time.Hour, func(context.Context) (int, error) { return 0, nil }).
		Map(func(v int) int { return v })

	spec := signal.Inspect()

	if spec.Kind != "signal" {
		t.Fatalf("expected kind signal, got %q", spec.Kind)
	}

	if spec.Metadata["initial_notify"] != "true" {
		t.Fatalf("expected initial_notify=true, got %q", spec.Metadata["initial_notify"])
	}
}

func TestSignal_RunClosesSubscriptionsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	signal := sup.NewSignal("count", 0)
	values := signal.Subscribe(ctx)
	updates := signal.Watch(ctx)
	done := make(chan error, 1)

	go func() {
		done <- signal.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal to stop")
	}

	select {
	case _, ok := <-values:
		if ok {
			t.Fatal("expected values channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for values channel to close")
	}

	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("expected watch channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watch channel to close")
	}
}

func TestSignal_RunReturnsSourceError(t *testing.T) {
	expectedErr := errors.New("source failed")
	signal := sup.NewSignal("count", 0).
		Source(sup.SourceFunc[int](func(context.Context, sup.EmitFunc[int]) error {
			return expectedErr
		}))

	err := signal.Run(t.Context())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected source error %v, got %v", expectedErr, err)
	}
}

type timerErrorProcessor struct {
	err error
}

func (p timerErrorProcessor) Process(ctx context.Context, value int, runtime sup.ProcessorRuntime[int]) error {
	runtime.After(time.Millisecond, 1)
	return nil
}

func (p timerErrorProcessor) OnTimer(ctx context.Context, timerID uint64, runtime sup.ProcessorRuntime[int]) error {
	return p.err
}

func TestSignal_RunReturnsTimerProcessorError(t *testing.T) {
	expectedErr := errors.New("timer failed")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	signal := sup.NewSignal("count", 0).
		Source(sup.SourceFunc[int](func(ctx context.Context, emit sup.EmitFunc[int]) error {
			<-ctx.Done()
			return nil
		})).
		Processor(timerErrorProcessor{err: expectedErr})
	done := make(chan error, 1)

	go func() {
		done <- signal.Run(ctx)
	}()

	if err := signal.Write(ctx, 1); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected timer processor error %v, got %v", expectedErr, err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for timer processor error")
	}
}
