package sup

import (
	"context"
)

// Actor represents a concurrent entity that can be supervised.
//
// The Run method should return an error if the actor needs to be restarted, or nil if it can exit cleanly.
// Panics will also trigger a restart.
type Actor interface {
	// ID returns the actor id.
	ID() string

	// Run starts the actor and blocks until it stops or fails.
	Run(context.Context) error

	// Inspect returns structured metadata for the actor.
	Inspect() Spec
}

type actorFunc struct {
	id string
	fn func(ctx context.Context) error
}

// ID returns the actor id.
func (a *actorFunc) ID() string {
	return a.id
}

// Run calls the wrapped actor function.
func (a *actorFunc) Run(ctx context.Context) error {
	return a.fn(ctx)
}

// Inspect returns the actor function spec.
func (a *actorFunc) Inspect() Spec {
	return Spec{
		Kind:         "actor_func",
		Dependencies: []string{},
		Metadata:     map[string]string{},
	}
}

// ActorFunc creates a simple stateless actor from a function.
func ActorFunc(id string, fn func(ctx context.Context) error) Actor {
	return &actorFunc{
		id: id,
		fn: fn,
	}
}
