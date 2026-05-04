package sup

import (
	"context"
	"io"
	"log/slog"
)

var noOpLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Actor represents a concurrent entity that can be supervised.
// It has a name and a Run method that executes its logic.
// The Run method should return an error if the actor needs to be restarted, or nil if it can exit cleanly.
// Panics will also trigger a restart.
// The setLogger method is used internally by the supervisor to inject a logger into the actor.
type Actor interface {
	Name() string
	Run(context.Context) error
	setLogger(*slog.Logger)
}

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

type actorFunc struct {
	name   string
	logger *slog.Logger
	fn     func(ctx context.Context, logger *slog.Logger) error
}

func (a *actorFunc) Name() string {
	return a.name
}

func (a *actorFunc) Run(ctx context.Context) error {
	return a.fn(ctx, a.logger)
}

func (a *actorFunc) setLogger(l *slog.Logger) {
	a.logger = l.With(slog.String("actor", a.name))
}

// ActorFunc creates a simple stateless actor from a function.
func ActorFunc(name string, fn func(ctx context.Context, logger *slog.Logger) error) Actor {
	return &actorFunc{
		name:   name,
		fn:     fn,
		logger: noOpLogger,
	}
}
