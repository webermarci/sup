package sup

import "log/slog"

// BaseActor provides a simple implementation of the Actor interface with a name and a logger.
// It can be embedded in other structs to create more complex actors.
// The Name and Logger methods are safe to call from inside Run(),
// and the setLogger method is used internally by the supervisor to inject a logger into the actor.
type BaseActor struct {
	name   string
	logger *slog.Logger
}

// NewBaseActor creates a new BaseActor with the given name.
// The logger is initialized to a no-op logger and will be set by the supervisor when the actor is spawned.
func NewBaseActor(name string) *BaseActor {
	return &BaseActor{
		name:   name,
		logger: noOpLogger,
	}
}

// Name returns the actor's name. It is safe to call from inside Run().
func (a *BaseActor) Name() string {
	return a.name
}

// Logger returns the actor's logger. It is safe to call from inside Run().
func (a *BaseActor) Logger() *slog.Logger {
	return a.logger
}

func (a *BaseActor) setLogger(logger *slog.Logger) {
	a.logger = logger.With("actor", a.name)
}
