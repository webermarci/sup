package sup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallInbox_Success(t *testing.T) {
	ctx := t.Context()
	inbox := NewCallInbox[string, int](10)
	msg := "calculate_length"

	go func() {
		req := <-inbox.Receive()
		if req.Payload() == msg {
			req.Reply(len(msg), nil)
		} else {
			req.Reply(0, errors.New("unexpected message"))
		}
	}()

	res, err := inbox.Call(ctx, msg)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res != 16 {
		t.Errorf("expected 16, got %d", res)
	}
}

func TestCallInbox_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	inbox := NewCallInbox[string, int](1)

	_, err := inbox.Call(ctx, "hang_me")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
}

func TestCallInbox_Closed(t *testing.T) {
	ctx := t.Context()
	inbox := NewCallInbox[string, int](1)
	inbox.Close()

	_, err := inbox.Call(ctx, "test")
	if !errors.Is(err, ErrCallInboxClosed) {
		t.Errorf("expected ErrCallInboxClosed, got %v", err)
	}
}

func TestCallInbox_Full(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	inbox := NewCallInbox[string, int](1)

	go func() {
		inbox.Call(context.Background(), "first")
	}()

	time.Sleep(10 * time.Millisecond)

	_, err := inbox.Call(ctx, "second")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context timeout due to full buffer, got %v", err)
	}
}

func TestCallInbox_ActorError(t *testing.T) {
	ctx := t.Context()
	inbox := NewCallInbox[string, string](1)
	expectedErr := errors.New("hardware_failure")

	go func() {
		req := <-inbox.Receive()
		req.Reply("", expectedErr)
	}()

	_, err := inbox.Call(ctx, "ping")

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestCallInbox_CloseSafety(t *testing.T) {
	inbox := NewCallInbox[int, int](1)

	inbox.Close()
	inbox.Close()

	if !inbox.closed.Load() {
		t.Error("expected closed to be true")
	}
}
