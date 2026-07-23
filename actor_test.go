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

func TestActorFuncHasDefaultSpec(t *testing.T) {
	actor := sup.ActorFunc("worker", func(context.Context) error {
		return nil
	})
	inspectable, ok := actor.(sup.Inspectable)
	if !ok {
		t.Fatal("expected actor function to be inspectable")
	}

	spec := inspectable.Inspect()
	if spec.Kind != "actor" {
		t.Fatalf("expected default kind actor, got %q", spec.Kind)
	}
	if len(spec.Dependencies) != 0 {
		t.Fatalf("expected no default dependencies, got %v", spec.Dependencies)
	}
	if len(spec.Metadata) != 0 {
		t.Fatalf("expected no default metadata, got %v", spec.Metadata)
	}
}

func TestActorFuncUsesConfiguredSpec(t *testing.T) {
	actor := sup.ActorFunc(
		"worker",
		func(context.Context) error { return nil },
		sup.WithSpec(sup.Spec{
			Kind:         "poller",
			Dependencies: []string{"input"},
			Metadata:     map[string]any{"interval": "1s"},
		}),
	)

	inspectable := actor.(sup.Inspectable)
	spec := inspectable.Inspect()
	if spec.Kind != "poller" {
		t.Fatalf("expected kind poller, got %q", spec.Kind)
	}
	if len(spec.Dependencies) != 1 || spec.Dependencies[0] != "input" {
		t.Fatalf("unexpected dependencies %v", spec.Dependencies)
	}
	if spec.Metadata["interval"] != "1s" {
		t.Fatalf("unexpected metadata %v", spec.Metadata)
	}
}

func TestActorFuncRejectsInvalidArguments(t *testing.T) {
	requirePanic(t, func() {
		sup.ActorFunc("", func(context.Context) error { return nil })
	})

	requirePanic(t, func() {
		sup.ActorFunc("worker", nil)
	})

	requirePanic(t, func() {
		sup.ActorFunc("worker", func(context.Context) error { return nil }, nil)
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
