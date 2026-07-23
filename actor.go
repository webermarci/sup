package sup

import "context"

// Actor represents a concurrent entity that can be supervised.
//
// Run must return nil when its context is canceled or the actor completes
// intentionally. It should return a non-nil error only for failures that may
// require a restart. Panics are recovered by Supervisor and treated as failures.
type Actor interface {
	// ID returns the actor id.
	ID() string

	// Run starts the actor and blocks until it stops or fails. Context
	// cancellation is a clean shutdown and must return nil.
	Run(context.Context) error
}

// Inspectable is implemented by actors that expose structured metadata.
// Inspection is optional and does not affect actor execution or supervision.
type Inspectable interface {
	Inspect() Spec
}

type actorFunc struct {
	id   string
	fn   func(ctx context.Context) error
	spec Spec
}

// ID returns the actor id.
func (a *actorFunc) ID() string {
	return a.id
}

// Run calls the wrapped actor function.
func (a *actorFunc) Run(ctx context.Context) error {
	err := a.fn(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// Inspect returns the actor function spec.
func (a *actorFunc) Inspect() Spec {
	return a.spec
}

// ActorFuncOption configures an actor created by ActorFunc.
type ActorFuncOption func(*actorFunc)

// WithSpec configures an actor function's structured inspection spec.
func WithSpec(spec Spec) ActorFuncOption {
	return func(actor *actorFunc) {
		actor.spec = spec
	}
}

// ActorFunc creates a simple stateless actor from a function.
func ActorFunc(
	id string,
	fn func(ctx context.Context) error,
	options ...ActorFuncOption,
) Actor {
	if id == "" {
		panic("sup: actor id cannot be empty")
	}
	if fn == nil {
		panic("sup: actor function cannot be nil")
	}

	actor := &actorFunc{
		id: id,
		fn: fn,
		spec: Spec{
			Kind:     "actor",
			Metadata: map[string]any{},
		},
	}
	for _, option := range options {
		if option == nil {
			panic("sup: actor function option cannot be nil")
		}
		option(actor)
	}
	return actor
}
