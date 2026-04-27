package sup

import "context"

type Actor interface {
	Name() string
	Run(context.Context) error
}

type BaseActor struct {
	name string
}

func NewBaseActor(name string) *BaseActor {
	return &BaseActor{name: name}
}

func (a *BaseActor) Name() string {
	return a.name
}

type actorFunc struct {
	name string
	fn   func(context.Context) error
}

func (a *actorFunc) Name() string {
	return a.name
}

func (a *actorFunc) Run(ctx context.Context) error {
	return a.fn(ctx)
}

func ActorFunc(name string, fn func(context.Context) error) Actor {
	return &actorFunc{name: name, fn: fn}
}
