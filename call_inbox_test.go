package sup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestCallInbox_Success(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCallInbox[string, int](10)
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

	inbox := sup.NewCallInbox[string, int](1)

	_, err := inbox.Call(ctx, "hang_me")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
}

func TestCallInbox_Closed(t *testing.T) {
	ctx := t.Context()
	inbox := sup.NewCallInbox[string, int](1)
	inbox.Close()

	_, err := inbox.Call(ctx, "test")
	if !errors.Is(err, sup.ErrCallInboxClosed) {
		t.Errorf("expected ErrCallInboxClosed, got %v", err)
	}
}

func TestCallInbox_Full(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	inbox := sup.NewCallInbox[string, int](1)

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
	inbox := sup.NewCallInbox[string, string](1)
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
	inbox := sup.NewCallInbox[int, int](1)

	inbox.Close()
	inbox.Close()

	if !inbox.Closed() {
		t.Error("expected closed to be true")
	}
}

func TestCallInbox_LenCap(t *testing.T) {
	inbox := sup.NewCallInbox[string, int](2)

	if got := inbox.Cap(); got != 2 {
		t.Fatalf("expected cap 2, got %d", got)
	}

	done := make(chan struct{})
	go func() {
		_, _ = inbox.Call(context.Background(), "first")
		close(done)
	}()

	deadline := time.After(time.Second)
	for inbox.Len() != 1 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for len 1, got %d", inbox.Len())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case req := <-inbox.Receive():
		req.Reply(1, nil)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for call to finish")
	}
}
