package sup

import "context"

// Actor represents a concurrent entity that can be supervised.
//
// Run must return nil when its context is canceled or the actor completes
// intentionally. It should return a non-nil error only for failures that may
// require a restart. Panics are recovered by supervisors and treated as failures.
//
// Each actor instance must belong to exactly one supervisor in a running tree.
// Applications must finish configuring the tree before running only its root
// supervisor. Supervisors invoke child Run methods and may invoke them again
// sequentially when restarting actors. Run must never be called concurrently
// on the same actor instance.
type Actor interface {
	// ID returns the actor id.
	ID() string

	// Run starts the actor and blocks until it stops or fails. Context
	// cancellation is a clean shutdown and must return nil.
	Run(context.Context) error
}

type actorFunc struct {
	id string
	fn func(ctx context.Context) error
}

var _ Actor = (*actorFunc)(nil)

// ID returns the actor id.
func (a *actorFunc) ID() string {
	return a.id
}

// Run calls the wrapped actor function.
func (a *actorFunc) Run(ctx context.Context) error {
	if err := a.fn(ctx); ctx.Err() != nil {
		return nil
	} else {
		return err
	}
}

// ActorFunc creates a simple stateless actor from a function.
func ActorFunc(
	id string,
	fn func(ctx context.Context) error,
) Actor {
	if id == "" {
		panic("sup: actor id cannot be empty")
	}
	return &actorFunc{
		id: id,
		fn: fn,
	}
}
