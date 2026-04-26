package bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestSignal_DefaultValue(t *testing.T) {
	signal := NewSignal(t.Name(), func() (int, error) {
		return 42, nil
	})

	go signal.Run(t.Context())

	if v := signal.Read(); v != 0 {
		t.Errorf("expected zero value, got %d", v)
	}
}

func TestSignal_InitialValue(t *testing.T) {
	signal := NewSignal(t.Name(), func() (int, error) {
		return 99, nil
	}).WithInitialValue(7)

	go signal.Run(t.Context())

	if v := signal.Read(); v != 7 {
		t.Errorf("expected initial value 7, got %d", v)
	}
}

func TestSignal_Value(t *testing.T) {
	signal := NewSignal(t.Name(), func() (int, error) {
		return 42, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(t.Context())

	time.Sleep(50 * time.Millisecond)

	if v := signal.Read(); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestSignal_ErrorSkipsUpdate(t *testing.T) {
	signal := NewSignal(t.Name(), func() (int, error) {
		return 0, errors.New("oops")
	}).
		WithInitialValue(5).
		WithInterval(10 * time.Millisecond)

	go signal.Run(t.Context())

	time.Sleep(50 * time.Millisecond)

	if v := signal.Read(); v != 5 {
		t.Errorf("expected value to stay at 5, got %d", v)
	}
}

func TestSignal_Subscribe(t *testing.T) {
	ctx := t.Context()

	signal := NewSignal(t.Name(), func() (int, error) {
		return 42, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for signal value")
	}
}

func TestSignal_MultipleSubscribers(t *testing.T) {
	ctx := t.Context()

	signal := NewSignal(t.Name(), func() (int, error) {
		return 55, nil
	}).WithInterval(10 * time.Millisecond)

	go signal.Run(ctx)

	ch1 := signal.Subscribe(ctx)
	ch2 := signal.Subscribe(ctx)

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

func TestSignal_UnsubscribeOnContextCancel(t *testing.T) {
	ctx := t.Context()

	signal := NewSignal(t.Name(), func() (int, error) {
		return 1, nil
	}).WithInterval(10 * time.Millisecond)

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

func TestSignal_InitialNotifyEnabled(t *testing.T) {
	ctx := t.Context()

	signal := NewSignal(t.Name(), func() (int, error) {
		return 0, nil
	}).
		WithInitialValue(42).
		WithInitialNotify(true).
		WithInterval(time.Hour)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 42 {
			t.Fatalf("expected initial notify value 42, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestSignal_InitialNotifyDisabled(t *testing.T) {
	ctx := t.Context()

	signal := NewSignal(t.Name(), func() (int, error) {
		return 0, nil
	}).
		WithInitialValue(42).
		WithInterval(time.Hour)

	go signal.Run(ctx)

	ch := signal.Subscribe(ctx)

	select {
	case v := <-ch:
		t.Errorf("expected no initial notification, got %d", v)
	case <-time.After(100 * time.Millisecond):
		// correct
	}
}

func TestSignal_ActorInterface(t *testing.T) {
	signal := NewSignal(t.Name(), func() (int, error) {
		return 0, errors.New("fail")
	})

	if _, ok := any(signal).(sup.Actor); !ok {
		t.Fatal("signal does not implement sup.Actor interface")
	}
}
