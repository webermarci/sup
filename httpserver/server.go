package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/webermarci/sup"
)

const defaultShutdownTimeout = 5 * time.Second

// Actor creates and serves a fresh net/http Server for each execution attempt.
type Actor struct {
	id              string
	server          func(context.Context) (*http.Server, error)
	serve           func(context.Context, *http.Server) error
	shutdownTimeout time.Duration
}

var _ sup.Actor = (*Actor)(nil)

// NewActor creates an HTTP server actor. The server function is called once
// for each execution attempt and must return a fresh *http.Server. Optional
// configuration is added with methods before the actor is run.
func NewActor(
	id string,
	server func(context.Context) (*http.Server, error),
) *Actor {
	if id == "" {
		panic("httpserver: actor id cannot be empty")
	}
	return &Actor{
		id:              id,
		server:          server,
		serve:           defaultServe,
		shutdownTimeout: defaultShutdownTimeout,
	}
}

// SetServeFunc configures how the HTTP server is started and returns the
// actor for chaining. By default the actor calls
// (*http.Server).ListenAndServe. The function must return when the server is
// shut down. Custom listeners must be recreated for every execution attempt.
func (a *Actor) SetServeFunc(
	serve func(context.Context, *http.Server) error,
) *Actor {
	a.serve = serve
	return a
}

// SetShutdownTimeout configures the maximum time allowed for graceful
// shutdown after the actor context is canceled and returns the actor for
// chaining. When graceful shutdown exceeds the timeout, the actor forcibly
// closes the server.
func (a *Actor) SetShutdownTimeout(timeout time.Duration) *Actor {
	if timeout <= 0 {
		panic("httpserver: shutdown timeout must be positive")
	}
	a.shutdownTimeout = timeout
	return a
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Run serves one HTTP server execution attempt. Context cancellation triggers
// graceful shutdown and is treated as a clean actor stop. http.ErrServerClosed
// is also treated as a clean stop.
func (a *Actor) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}

	server, err := a.server(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	served := make(chan error, 1)
	go func() {
		served <- a.serve(ctx, server)
	}()

	select {
	case err := <-served:
		return normalizeServeError(ctx, err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.shutdownTimeout)
		shutdownErr := server.Shutdown(shutdownCtx)
		cancel()
		if shutdownErr != nil {
			_ = server.Close()
		}
		return nil
	}
}

func defaultServe(_ context.Context, server *http.Server) error {
	return server.ListenAndServe()
}

func normalizeServeError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
