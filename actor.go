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
