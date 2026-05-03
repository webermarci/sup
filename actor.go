package sup

import (
	"context"
	"io"
	"log/slog"
)

var noOpLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type Actor interface {
	Name() string
	Run(context.Context) error
	setLogger(*slog.Logger)
}

type BaseActor struct {
	name   string
	logger *slog.Logger
}

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
