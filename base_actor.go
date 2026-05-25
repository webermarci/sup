package sup

import "log/slog"

// BaseActor provides a simple implementation of the Actor interface with a id and a logger.
// It can be embedded in other structs to create more complex actors.
// The ID and Logger methods are safe to call from inside Run(),
// and the setLogger method is used internally by the supervisor to inject a logger into the actor.
type BaseActor struct {
	id     string
	logger *slog.Logger
}

// NewBaseActor creates a new BaseActor with the given id.
// The logger is initialized to a no-op logger and will be set by the supervisor when the actor is spawned.
func NewBaseActor(id string) *BaseActor {
	return &BaseActor{
		id:     id,
		logger: noOpLogger,
	}
}

// ID returns the actor's id. It is safe to call from inside Run().
func (a *BaseActor) ID() string {
	return a.id
}

// Logger returns the actor's logger. It is safe to call from inside Run().
func (a *BaseActor) Logger() *slog.Logger {
	return a.logger
}

// Inspect returns the specification.
func (a *BaseActor) Inspect() Spec {
	return Spec{
		Kind:         "actor",
		Dependencies: []string{},
		Metadata:     map[string]string{},
	}
}

func (a *BaseActor) setLogger(logger *slog.Logger) {
	a.logger = logger.With("actor", a.id)
}
