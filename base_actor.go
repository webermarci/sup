package sup

// BaseActor provides a reusable actor identity and default inspection metadata.
type BaseActor struct {
	id string
}

// NewBaseActor creates a base actor with the given id.
func NewBaseActor(id string) *BaseActor {
	return &BaseActor{
		id: id,
	}
}

// ID returns the actor id.
func (a *BaseActor) ID() string {
	return a.id
}

// Inspect returns the default actor spec.
func (a *BaseActor) Inspect() Spec {
	return Spec{
		Kind:         "actor",
		Dependencies: []string{},
		Metadata:     map[string]string{},
	}
}
