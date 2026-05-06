package sup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPushedSignal_DefaultValue(t *testing.T) {
	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	})

	go signal.Run(t.Context())

	if v := signal.Read(); v != 0 {
		t.Errorf("expected zero value, got %d", v)
	}
}

func TestPushedSignal_InitialValue(t *testing.T) {
	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	}).WithInitialValue(42)

	go signal.Run(t.Context())

	if v := signal.Read(); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestPushedSignal_SetValue(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	})

	go signal.Run(ctx)

	if err := signal.Write(ctx, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := signal.Read(); v != 10 {
		t.Errorf("expected 10, got %d", v)
	}
}

func TestPushedSignal_SetValueRejected(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return errors.New("rejected")
	}).WithInitialValue(5)

	go signal.Run(ctx)

	if err := signal.Write(ctx, 99); err == nil {
		t.Fatal("expected error, got nil")
	}

	if v := signal.Read(); v != 5 {
		t.Errorf("expected value to remain 5, got %d", v)
	}
}

func TestPushedSignal_Subscribe(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	})
	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)
	signal.Write(ctx, 77)

	select {
	case v := <-ch:
		if v != 77 {
			t.Errorf("expected 77, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for value")
	}
}

func TestPushedSignal_SubscribeNotNotifiedOnError(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return errors.New("rejected")
	})
	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)
	signal.Write(ctx, 99)

	select {
	case v := <-ch:
		t.Errorf("expected no notification on rejected set, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPushedSignal_MultipleSubscribers(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	})
	go signal.Run(ctx)

	ch1 := signal.Subscribe(ctx)
	ch2 := signal.Subscribe(ctx)
	signal.Write(ctx, 55)

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

func TestPushedSignal_UnsubscribeOnContextCancel(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	})
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

func TestPushedSignal_InitialNotifyEnabled(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	}).
		WithInitialValue(99).
		WithInitialNotify(true)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 99 {
			t.Fatalf("expected initial notify value 99, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestPushedSignal_InitialNotifyDisabled(t *testing.T) {
	ctx := t.Context()

	signal := NewPushedSignal(t.Name(), func(_ context.Context, v int) error {
		return nil
	}).WithInitialValue(99)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		t.Errorf("expected no initial notification, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPushedSignal_ActorInterface(t *testing.T) {
	signal := NewPushedSignal(t.Name(), func(_ context.Context, value int) error {
		return nil
	})

	if _, ok := any(signal).(Actor); !ok {
		t.Errorf("signal does not implement sup.Actor interface")
	}
}
