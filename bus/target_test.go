package bus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTarget_DefaultValue(t *testing.T) {
	target := NewTarget(func(v int) error {
		return nil
	})

	go target.Run(t.Context())

	if v := target.Value(); v != 0 {
		t.Errorf("expected zero value, got %d", v)
	}
}

func TestTarget_InitialValue(t *testing.T) {
	target := NewTarget(func(v int) error {
		return nil
	}).WithInitialValue(42)

	go target.Run(t.Context())

	if v := target.Value(); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestTarget_SetValue(t *testing.T) {
	target := NewTarget(func(v int) error {
		return nil
	})

	go target.Run(t.Context())

	if err := target.SetValue(10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := target.Value(); v != 10 {
		t.Errorf("expected 10, got %d", v)
	}
}

func TestTarget_SetValueRejected(t *testing.T) {
	target := NewTarget(func(v int) error {
		return errors.New("rejected")
	}).WithInitialValue(5)

	go target.Run(t.Context())

	if err := target.SetValue(99); err == nil {
		t.Fatal("expected error, got nil")
	}

	if v := target.Value(); v != 5 {
		t.Errorf("expected value to remain 5, got %d", v)
	}
}

func TestTarget_Subscribe(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error { return nil })
	go target.Run(ctx)

	ch := target.Subscribe(ctx)
	target.SetValue(77)

	select {
	case v := <-ch:
		if v != 77 {
			t.Errorf("expected 77, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for value")
	}
}

func TestTarget_SubscribeNotNotifiedOnError(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error {
		return errors.New("rejected")
	})
	go target.Run(ctx)

	ch := target.Subscribe(ctx)
	target.SetValue(99)

	select {
	case v := <-ch:
		t.Errorf("expected no notification on rejected set, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTarget_MultipleSubscribers(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error { return nil })
	go target.Run(ctx)

	ch1 := target.Subscribe(ctx)
	ch2 := target.Subscribe(ctx)
	target.SetValue(55)

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

func TestTarget_UnsubscribeOnContextCancel(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error { return nil })
	go target.Run(ctx)

	subCtx, subCancel := context.WithCancel(ctx)
	ch := target.Subscribe(subCtx)
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

func TestTarget_Sync(t *testing.T) {
	synced := make(chan int, 1)
	target := NewTarget(func(v int) error {
		synced <- v
		return nil
	}).WithInitialValue(3)

	go target.Run(t.Context())

	if err := target.Sync(); err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}

	select {
	case v := <-synced:
		if v != 3 {
			t.Errorf("expected sync with value 3, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for sync")
	}
}

func TestTarget_SyncNotifiesSubscribers(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error {
		return nil
	}).WithInitialValue(3)

	go target.Run(ctx)

	ch := target.Subscribe(ctx)

	if err := target.Sync(); err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}

	select {
	case v := <-ch:
		if v != 3 {
			t.Errorf("expected subscriber to receive 3 on sync, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for sync notification")
	}
}

func TestTarget_SyncError(t *testing.T) {
	target := NewTarget(func(v int) error {
		return errors.New("sync failed")
	})

	go target.Run(t.Context())

	if err := target.Sync(); err == nil {
		t.Fatal("expected sync error, got nil")
	}
}

func TestTarget_InitialNotifyEnabled(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error {
		return nil
	}).
		WithInitialValue(99).
		WithInitialNotify(true)

	go target.Run(ctx)

	ch := target.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 99 {
			t.Fatalf("expected initial notify value 99, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestTarget_InitialNotifyDisabled(t *testing.T) {
	ctx := t.Context()

	target := NewTarget(func(v int) error {
		return nil
	}).WithInitialValue(99)

	go target.Run(ctx)

	ch := target.Subscribe(ctx)

	select {
	case v := <-ch:
		t.Errorf("expected no initial notification, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}
