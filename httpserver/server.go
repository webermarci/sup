package httpserver

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
)

const defaultShutdownTimeout = 5 * time.Second

var (
	// ErrActorRunning is returned when Run is called while the actor is already
	// serving a server.
	ErrActorRunning = errors.New("httpserver: actor is already running")

	// ErrNilServer is returned when a ServerFunc returns a nil server without an
	// error.
	ErrNilServer = errors.New("httpserver: server function returned nil")
)

// ServerFunc creates a fresh HTTP server for one execution attempt.
//
// A new server must be returned for every attempt because net/http servers
// cannot be reused after Shutdown has been called.
type ServerFunc func(context.Context) (*http.Server, error)

// ServeFunc runs one HTTP server execution attempt. The function should return
// when the server has been shut down. It is called once for every Run attempt.
// For custom listeners, create a fresh listener inside the function so that
// supervisor restarts can serve again.
type ServeFunc func(context.Context, *http.Server) error

// Option configures an HTTP server actor during construction.
type Option func(*Actor)

// WithServe configures how the HTTP server is started. By default the actor
// calls (*http.Server).ListenAndServe.
func WithServe(serve ServeFunc) Option {
	if serve == nil {
		panic("httpserver: serve function cannot be nil")
	}

	return func(actor *Actor) {
		actor.serve = serve
	}
}

// WithShutdownTimeout configures the maximum time allowed for graceful
// shutdown after the actor context is canceled.
func WithShutdownTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		panic("httpserver: shutdown timeout must be positive")
	}

	return func(actor *Actor) {
		actor.shutdownTimeout = timeout
	}
}

// Actor creates and serves a fresh net/http Server for each execution attempt.
// It is safe to call Run sequentially and rejects concurrent calls.
type Actor struct {
	id              string
	server          ServerFunc
	serve           ServeFunc
	shutdownTimeout time.Duration
	running         atomic.Bool
}

var _ sup.Actor = (*Actor)(nil)
var _ sup.Inspectable = (*Actor)(nil)

// NewActor creates an HTTP server actor. The server function is called once
// for each execution attempt and must return a fresh *http.Server.
func NewActor(id string, server ServerFunc, options ...Option) *Actor {
	if id == "" {
		panic("httpserver: actor id cannot be empty")
	}
	if server == nil {
		panic("httpserver: server function cannot be nil")
	}

	actor := &Actor{
		id:              id,
		server:          server,
		serve:           defaultServe,
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, option := range options {
		if option == nil {
			panic("httpserver: option cannot be nil")
		}
		option(actor)
	}

	return actor
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Inspect returns the HTTP server actor spec.
func (a *Actor) Inspect() sup.Spec {
	return sup.Spec{
		Kind: "http_server",
		Metadata: map[string]any{
			"shutdown_timeout": a.shutdownTimeout,
		},
	}
}

// Run serves one HTTP server execution attempt. Context cancellation triggers
// graceful shutdown and is treated as a clean actor stop. http.ErrServerClosed
// is also treated as a clean stop.
func (a *Actor) Run(ctx context.Context) error {
	if ctx == nil {
		panic("httpserver: context cannot be nil")
	}
	if !a.running.CompareAndSwap(false, true) {
		return ErrActorRunning
	}
	defer a.running.Store(false)

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
	if server == nil {
		return ErrNilServer
	}

	served := make(chan error, 1)
	go func() {
		served <- a.serve(ctx, server)
	}()

	select {
	case err := <-served:
		return normalizeServeError(ctx, err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		_ = server.Shutdown(shutdownCtx)
		cancel()
		<-served
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
