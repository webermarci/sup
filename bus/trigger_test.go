package bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestTrigger_DefaultValue(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	})

	go trigger.Run(t.Context())

	if v := trigger.Read(); v != 0 {
		t.Errorf("expected zero value, got %d", v)
	}
}

func TestTrigger_InitialValue(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	}).WithInitialValue(42)

	go trigger.Run(t.Context())

	if v := trigger.Read(); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestTrigger_SetValue(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	})

	go trigger.Run(t.Context())

	if err := trigger.Write(10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := trigger.Read(); v != 10 {
		t.Errorf("expected 10, got %d", v)
	}
}

func TestTrigger_SetValueRejected(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(v int) error {
		return errors.New("rejected")
	}).WithInitialValue(5)

	go trigger.Run(t.Context())

	if err := trigger.Write(99); err == nil {
		t.Fatal("expected error, got nil")
	}

	if v := trigger.Read(); v != 5 {
		t.Errorf("expected value to remain 5, got %d", v)
	}
}

func TestTrigger_Subscribe(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	})
	go trigger.Run(ctx)

	ch := trigger.Subscribe(ctx)
	trigger.Write(77)

	select {
	case v := <-ch:
		if v != 77 {
			t.Errorf("expected 77, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for value")
	}
}

func TestTrigger_SubscribeNotNotifiedOnError(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return errors.New("rejected")
	})
	go trigger.Run(ctx)

	ch := trigger.Subscribe(ctx)
	trigger.Write(99)

	select {
	case v := <-ch:
		t.Errorf("expected no notification on rejected set, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTrigger_MultipleSubscribers(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	})
	go trigger.Run(ctx)

	ch1 := trigger.Subscribe(ctx)
	ch2 := trigger.Subscribe(ctx)
	trigger.Write(55)

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

func TestTrigger_UnsubscribeOnContextCancel(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	})
	go trigger.Run(ctx)

	subCtx, subCancel := context.WithCancel(ctx)
	ch := trigger.Subscribe(subCtx)
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

func TestTrigger_Sync(t *testing.T) {
	synced := make(chan int, 1)
	trigger := NewTrigger(t.Name(), func(v int) error {
		synced <- v
		return nil
	}).WithInitialValue(3)

	go trigger.Run(t.Context())

	if err := trigger.Sync(); err != nil {
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

func TestTrigger_SyncNotifiesSubscribers(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	}).WithInitialValue(3)

	go trigger.Run(ctx)

	ch := trigger.Subscribe(ctx)

	if err := trigger.Sync(); err != nil {
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

func TestTrigger_SyncError(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(v int) error {
		return errors.New("sync failed")
	})

	go trigger.Run(t.Context())

	if err := trigger.Sync(); err == nil {
		t.Fatal("expected sync error, got nil")
	}
}

func TestTrigger_InitialNotifyEnabled(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	}).
		WithInitialValue(99).
		WithInitialNotify(true)

	go trigger.Run(ctx)

	ch := trigger.Subscribe(ctx)

	select {
	case v := <-ch:
		if v != 99 {
			t.Fatalf("expected initial notify value 99, got %d", v)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial notify")
	}
}

func TestTrigger_InitialNotifyDisabled(t *testing.T) {
	ctx := t.Context()

	trigger := NewTrigger(t.Name(), func(v int) error {
		return nil
	}).WithInitialValue(99)

	go trigger.Run(ctx)

	ch := trigger.Subscribe(ctx)

	select {
	case v := <-ch:
		t.Errorf("expected no initial notification, got %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTrigger_ActorInterface(t *testing.T) {
	trigger := NewTrigger(t.Name(), func(value int) error {
		return nil
	})

	if _, ok := any(trigger).(sup.Actor); !ok {
		t.Errorf("tigger does not implement sup.Actor interface")
	}
}
