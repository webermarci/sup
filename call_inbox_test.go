package sup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestCallInbox(t *testing.T) {
	inbox := sup.NewCallInbox[string, int](1)
	go func() {
		req := <-inbox.Receive()
		req.Reply(len(req.Payload()), nil)
	}()

	got, err := inbox.Call(t.Context(), "calculate_length")
	if err != nil {
		t.Fatal(err)
	}
	if got != 16 {
		t.Fatalf("expected 16, got %d", got)
	}
}

func TestCallInboxTryCall(t *testing.T) {
	inbox := sup.NewCallInbox[string, int](1)
	go func() {
		req := <-inbox.Receive()
		req.Reply(len(req.Payload()), nil)
	}()

	got, err := inbox.TryCall(t.Context(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestCallInboxReplyError(t *testing.T) {
	inbox := sup.NewCallInbox[string, string](1)
	expectedErr := errors.New("hardware failure")
	go func() {
		(<-inbox.Receive()).Reply("", expectedErr)
	}()

	if _, err := inbox.Call(t.Context(), "ping"); !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestCallInboxWaitingForReplyCanBeCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	inbox := sup.NewCallInbox[string, int](1)
	queued := make(chan struct{})
	go func() {
		<-inbox.Receive()
		close(queued)
	}()
	go func() {
		<-queued
		cancel()
	}()

	if _, err := inbox.Call(ctx, "no reply"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestCallRequestExposesCallerContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "value")
	inbox := sup.NewCallInbox[string, string](1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := <-inbox.Receive()
		if got := req.Context().Value(key{}); got != "value" {
			t.Errorf("expected caller context value, got %v", got)
		}
		req.Reply("ok", nil)
	}()

	if got, err := inbox.Call(ctx, "request"); err != nil || got != "ok" {
		t.Fatalf("expected reply ok, got %q, %v", got, err)
	}
	<-done
}

func TestCallRequestReplyOnlySucceedsOnce(t *testing.T) {
	inbox := sup.NewCallInbox[string, string](1)
	received := make(chan struct{})
	go func() {
		req := <-inbox.Receive()
		if !req.Reply("first", nil) {
			t.Error("expected first reply to succeed")
		}
		if req.Reply("second", nil) {
			t.Error("expected second reply to fail")
		}
		close(received)
	}()

	if got, err := inbox.Call(t.Context(), "request"); err != nil || got != "first" {
		t.Fatalf("expected first reply, got %q, %v", got, err)
	}
	<-received
}

func TestCallRequestReplyFailsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	inbox := sup.NewCallInbox[string, string](1)
	reqCh := make(chan sup.CallRequest[string, string], 1)
	go func() { reqCh <- <-inbox.Receive() }()

	done := make(chan error, 1)
	go func() {
		_, err := inbox.Call(ctx, "request")
		done <- err
	}()
	req := <-reqCh
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if req.Reply("late", nil) {
		t.Fatal("expected late reply to fail")
	}
}

func TestZeroCallRequestReplyFails(t *testing.T) {
	var req sup.CallRequest[string, string]
	if req.Context() != context.Background() {
		t.Fatal("expected zero request to use a background context")
	}
	if req.Reply("reply", nil) {
		t.Fatal("expected zero request reply to fail")
	}
}

func TestCallInboxErrors(t *testing.T) {
	inbox := sup.NewCallInbox[string, int](1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go inbox.Call(ctx, "first")
	deadline := time.Now().Add(time.Second)
	for len(inbox.Receive()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := inbox.TryCall(t.Context(), "second"); !errors.Is(err, sup.ErrInboxFull) {
		t.Fatalf("expected ErrInboxFull, got %v", err)
	}
}
