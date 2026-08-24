package sup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/webermarci/sup"
)

type minimalActor struct{}

func (minimalActor) ID() string {
	return "minimal"
}

func (minimalActor) Run(context.Context) error {
	return nil
}

func TestActorRequiresOnlyIDAndRun(t *testing.T) {
	var actor sup.Actor = minimalActor{}
	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

}

func TestActorFuncCancellationWinsOverReturnedError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	expectedErr := errors.New("failure during shutdown")
	actor := sup.ActorFunc("worker", func(context.Context) error {
		cancel()
		return expectedErr
	})

	if err := actor.Run(ctx); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestActorFuncRejectsInvalidArguments(t *testing.T) {
	requirePanic(t, func() {
		sup.ActorFunc("", func(context.Context) error { return nil })
	})

}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
