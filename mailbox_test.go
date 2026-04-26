package sup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestMailbox_TryCastAndClose(t *testing.T) {
	mb := sup.NewMailbox(2)

	if mb.IsClosed() {
		t.Fatal("expected mailbox to be open")
	}

	if err := sup.TryCast(mb, 1); err != nil {
		t.Fatalf("expected TryCast to succeed, got %v", err)
	}

	if err := sup.TryCast(mb, 2); err != nil {
		t.Fatalf("expected TryCast to succeed, got %v", err)
	}

	if err := sup.TryCast(mb, 3); !errors.Is(err, sup.ErrMailboxFull) {
		t.Fatalf("expected ErrMailboxFull, got %v", err)
	}

	val := <-mb.Receive()
	switch v := val.(type) {
	case sup.CastRequest[int]:
		if v.Payload() != 1 {
			t.Fatalf("expected message 1, got %d", v.Payload())
		}
	default:
		t.Fatalf("expected CastRequest[int], got %T", val)
	}

	mb.Close()

	if !mb.IsClosed() {
		t.Fatal("expected mailbox to be closed")
	}

	if err := sup.TryCast(mb, 4); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed, got %v", err)
	}

	val = <-mb.Receive()
	switch v := val.(type) {
	case sup.CastRequest[int]:
		if v.Payload() != 2 {
			t.Fatalf("expected message 2, got %d", v.Payload())
		}
	default:
		t.Fatalf("expected CastRequest[int], got %T", val)
	}

	if _, ok := <-mb.Receive(); ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestMailbox_CastContext_Timeout(t *testing.T) {
	mb := sup.NewMailbox(0)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := sup.CastContext(ctx, mb, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestMailbox_Cast_BlocksUntilReceiverReady(t *testing.T) {
	mb := sup.NewMailbox(0)

	done := make(chan error, 1)
	go func() {
		done <- sup.Cast(mb, 42)
	}()

	select {
	case err := <-done:
		t.Fatalf("cast should block, returned early with %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case v := <-mb.Receive():
		switch msg := v.(type) {
		case sup.CastRequest[int]:
			if msg.Payload() != 42 {
				t.Fatalf("expected 42, got %d", msg.Payload())
			}
		default:
			t.Fatalf("expected CastRequest[int], got %T", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for message")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected Cast to succeed, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cast did not complete after receiver read")
	}
}

func TestMailbox_Cast_OnClosedMailbox(t *testing.T) {
	mb := sup.NewMailbox(1)
	mb.Close()

	if err := sup.Cast(mb, 1); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed from Cast, got %v", err)
	}

	if err := sup.CastContext(t.Context(), mb, 1); !errors.Is(err, sup.ErrMailboxClosed) {
		t.Fatalf("expected ErrMailboxClosed from CastContext, got %v", err)
	}
}

func TestMailbox_Close_Idempotent(t *testing.T) {
	mb := sup.NewMailbox(1)
	mb.Close()
	mb.Close() // must not panic
}
