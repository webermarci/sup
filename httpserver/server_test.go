package httpserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup/httpserver"
)

func TestActorServesAndShutsDownGracefully(t *testing.T) {
	started := make(chan string, 1)
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})

	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				select {
				case <-requestStarted:
				default:
					close(requestStarted)
				}
				<-releaseRequest
				w.WriteHeader(http.StatusNoContent)
			}),
		}, nil
	},
		httpserver.WithShutdownTimeout(time.Second),
		httpserver.WithServe(func(_ context.Context, server *http.Server) error {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			started <- "http://" + listener.Addr().String()
			return server.Serve(listener)
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	baseURL := <-started
	requestDone := make(chan error, 1)
	go func() {
		response, err := http.Get(baseURL)
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	<-requestStarted

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned before active request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseRequest)
	if err := <-requestDone; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestActorNormalizesServerClosed(t *testing.T) {
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}, httpserver.WithServe(func(context.Context, *http.Server) error {
		return http.ErrServerClosed
	}))

	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("expected clean stop, got %v", err)
	}
}

func TestActorReturnsServeError(t *testing.T) {
	want := errors.New("serve failed")
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}, httpserver.WithServe(func(context.Context, *http.Server) error {
		return want
	}))

	if err := actor.Run(t.Context()); !errors.Is(err, want) {
		t.Fatalf("expected serve error %v, got %v", want, err)
	}
}

func TestActorReturnsFactoryError(t *testing.T) {
	want := errors.New("factory failed")
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return nil, want
	})

	if err := actor.Run(t.Context()); !errors.Is(err, want) {
		t.Fatalf("expected factory error %v, got %v", want, err)
	}
}

func TestActorRejectsNilServer(t *testing.T) {
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return nil, nil
	})

	if err := actor.Run(t.Context()); !errors.Is(err, httpserver.ErrNilServer) {
		t.Fatalf("expected ErrNilServer, got %v", err)
	}
}

func TestActorCreatesFreshServerForEveryRun(t *testing.T) {
	var factories atomic.Int32
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		factories.Add(1)
		return &http.Server{}, nil
	}, httpserver.WithServe(func(context.Context, *http.Server) error {
		return http.ErrServerClosed
	}))

	for range 2 {
		if err := actor.Run(t.Context()); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	}
	if got := factories.Load(); got != 2 {
		t.Fatalf("expected two server factories, got %d", got)
	}
}

func TestActorRejectsConcurrentRun(t *testing.T) {
	started := make(chan struct{})
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}, httpserver.WithServe(func(ctx context.Context, _ *http.Server) error {
		close(started)
		<-ctx.Done()
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()
	<-started

	if err := actor.Run(t.Context()); !errors.Is(err, httpserver.ErrActorRunning) {
		t.Fatalf("expected ErrActorRunning, got %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestActorCanceledContextDoesNotCreateServer(t *testing.T) {
	var factories atomic.Int32
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		factories.Add(1)
		return &http.Server{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := actor.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := factories.Load(); got != 0 {
		t.Fatalf("expected no factory call, got %d", got)
	}
}

func TestActorValidation(t *testing.T) {
	server := func(context.Context) (*http.Server, error) { return &http.Server{}, nil }
	requirePanic(t, func() { httpserver.NewActor("", server) })
	requirePanic(t, func() { httpserver.NewActor("server", nil) })
	requirePanic(t, func() { httpserver.NewActor("server", server, nil) })
	requirePanic(t, func() { httpserver.NewActor("server", server, httpserver.WithServe(nil)) })
	requirePanic(t, func() { httpserver.NewActor("server", server, httpserver.WithShutdownTimeout(0)) })
}

func TestActorInspect(t *testing.T) {
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}, httpserver.WithShutdownTimeout(2*time.Second))

	spec := actor.Inspect()
	if spec.Kind != "http_server" {
		t.Fatalf("expected http_server kind, got %q", spec.Kind)
	}
	if got := spec.Metadata["shutdown_timeout"]; got != 2*time.Second {
		t.Fatalf("expected shutdown timeout, got %v", got)
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
