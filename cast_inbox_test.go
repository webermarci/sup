package sup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestCastInbox_Success(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCastInbox[string](10)
	msg := "hello modbus"

	// Cast the message
	err := inbox.Cast(ctx, msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify it was received
	select {
	case received := <-inbox.Receive():
		if received != msg {
			t.Errorf("expected %s, got %s", msg, received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for message")
	}
}

func TestCastInbox_TryCast(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCastInbox[int](1)

	// Fill the buffer
	if err := inbox.TryCast(ctx, 1); err != nil {
		t.Fatalf("first trycast failed: %v", err)
	}

	// Second one should fail immediately because buffer is full (size 1)
	err := inbox.TryCast(ctx, 2)
	if !errors.Is(err, sup.ErrCastInboxFull) {
		t.Errorf("expected ErrCastInboxFull, got %v", err)
	}
}

func TestCastInbox_ContextCancellation(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCastInbox[int](1)

	// Fill the buffer so the next one blocks
	_ = inbox.Cast(ctx, 1)

	ctx, cancel := context.WithCancel(ctx)
	// Cancel the context immediately
	cancel()

	err := inbox.Cast(ctx, 2)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCastInbox_Closed(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCastInbox[int](5)
	inbox.Close()

	err := inbox.Cast(ctx, 1)
	if !errors.Is(err, sup.ErrCastInboxClosed) {
		t.Errorf("expected ErrCastInboxClosed, got %v", err)
	}

	// Verify Receive channel is closed
	_, ok := <-inbox.Receive()
	if ok {
		t.Error("expected receive channel to be closed")
	}
}

func TestCastInbox_LenCap(t *testing.T) {
	ctx := t.Context()
	size := 5
	inbox := sup.NewCastInbox[int](size)

	if inbox.Cap() != size {
		t.Errorf("expected cap %d, got %d", size, inbox.Cap())
	}

	_ = inbox.Cast(ctx, 1)
	_ = inbox.Cast(ctx, 2)

	if inbox.Len() != 2 {
		t.Errorf("expected len 2, got %d", inbox.Len())
	}
}

func TestCastInbox_ConcurrentSafety(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCastInbox[int](100)

	const workers = 10
	const msgsPerWorker = 100

	for range workers {
		go func() {
			for i := range msgsPerWorker {
				_ = inbox.Cast(ctx, i)
			}
		}()
	}

	count := 0
	for count < workers*msgsPerWorker {
		<-inbox.Receive()
		count++
	}
}

func TestCastInbox_CloseSafety(t *testing.T) {
	inbox := sup.NewCastInbox[int](1)

	inbox.Close()
	inbox.Close()

	if !inbox.Closed() {
		t.Error("expected closed to be true")
	}
}

func TestCastInbox_TryCastCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	inbox := sup.NewCastInbox[int](1)
	err := inbox.TryCast(ctx, 1)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCastInbox_TryCastClosed(t *testing.T) {
	inbox := sup.NewCastInbox[int](1)
	inbox.Close()

	err := inbox.TryCast(t.Context(), 1)
	if !errors.Is(err, sup.ErrCastInboxClosed) {
		t.Errorf("expected ErrCastInboxClosed, got %v", err)
	}
}
