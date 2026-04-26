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
