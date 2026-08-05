package sup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/webermarci/sup"
)

func TestCastInbox(t *testing.T) {
	inbox := sup.NewCastInbox[string]()
	received := make(chan string, 1)
	go func() { received <- <-inbox.Receive() }()

	if err := inbox.Cast(t.Context(), "hello"); err != nil {
		t.Fatal(err)
	}
	if got := <-received; got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestCastInboxCancellation(t *testing.T) {
	inbox := sup.NewCastInbox[int]()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := inbox.Cast(ctx, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
