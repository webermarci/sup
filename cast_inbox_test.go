package sup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/webermarci/sup"
)

func TestCastInbox(t *testing.T) {
	inbox := sup.NewCastInbox[string](1)
	if err := inbox.Cast(t.Context(), "hello"); err != nil {
		t.Fatal(err)
	}
	if got := <-inbox.Receive(); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestCastInboxErrors(t *testing.T) {
	inbox := sup.NewCastInbox[int](1)
	if err := inbox.TryCast(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := inbox.TryCast(t.Context(), 2); !errors.Is(err, sup.ErrInboxFull) {
		t.Fatalf("expected ErrInboxFull, got %v", err)
	}
}

func TestCastInboxCancellation(t *testing.T) {
	inbox := sup.NewCastInbox[int](1)
	if err := inbox.Cast(t.Context(), 1); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, send := range []func() error{
		func() error { return inbox.Cast(ctx, 2) },
		func() error { return inbox.TryCast(ctx, 2) },
	} {
		if err := send(); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	}
}
