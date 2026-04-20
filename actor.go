package sup

import "context"

type Actor interface {
	Run(context.Context) error
}

type ActorFunc func(context.Context) error

func (f ActorFunc) Run(ctx context.Context) error {
	return f(ctx)
}
