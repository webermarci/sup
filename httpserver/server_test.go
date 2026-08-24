package httpserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup/httpserver"
)

func TestActorServesAndShutsDownGracefully(t *testing.T) {
	started := make(chan string, 1)
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }
	defer release()

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
	}).
		SetShutdownTimeout(time.Second).
		SetServeFunc(func(_ context.Context, server *http.Server) error {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			started <- "http://" + listener.Addr().String()
			return server.Serve(listener)
		})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	var baseURL string
	select {
	case baseURL = <-started:
	case err := <-done:
		t.Fatalf("Run returned before the listener started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the listener to start")
	}

	requestDone := make(chan error, 1)
	go func() {
		response, err := http.Get(baseURL)
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-requestStarted:
	case err := <-requestDone:
		t.Fatalf("request finished before reaching the handler: %v", err)
	case err := <-done:
		t.Fatalf("Run returned before the request reached the handler: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the request to reach the handler")
	}

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned before active request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	if err := <-requestDone; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestActorForciblyClosesServerAfterShutdownTimeout(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	defer close(releaseRequest)
	testServer := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	}))

	serveStarted := make(chan struct{})
	shutdownStarted := make(chan struct{})
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return testServer.Config, nil
	}).
		SetShutdownTimeout(10 * time.Millisecond).
		SetServeFunc(func(_ context.Context, server *http.Server) error {
			server.RegisterOnShutdown(func() { close(shutdownStarted) })
			close(serveStarted)
			<-shutdownStarted
			return http.ErrServerClosed
		})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()
	select {
	case <-serveStarted:
	case err := <-done:
		t.Fatalf("Run returned before serving started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serving to start")
	}

	requestDone := make(chan error, 1)
	go func() {
		response, err := testServer.Client().Get(testServer.URL)
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-requestStarted:
	case err := <-requestDone:
		t.Fatalf("request finished before reaching the handler: %v", err)
	case err := <-done:
		t.Fatalf("Run returned before the request reached the handler: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the request to reach the handler")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the shutdown timeout")
	}

	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("expected the active request connection to be forcibly closed")
		}
	case <-time.After(time.Second):
		t.Fatal("active request connection was not forcibly closed")
	}
}

func TestActorNormalizesServerClosed(t *testing.T) {
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}).SetServeFunc(func(context.Context, *http.Server) error {
		return http.ErrServerClosed
	})

	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("expected clean stop, got %v", err)
	}
}

func TestActorReturnsServeError(t *testing.T) {
	want := errors.New("serve failed")
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		return &http.Server{}, nil
	}).SetServeFunc(func(context.Context, *http.Server) error {
		return want
	})

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

func TestActorCreatesFreshServerForEveryRun(t *testing.T) {
	var factories atomic.Int32
	actor := httpserver.NewActor("server", func(context.Context) (*http.Server, error) {
		factories.Add(1)
		return &http.Server{}, nil
	}).SetServeFunc(func(context.Context, *http.Server) error {
		return http.ErrServerClosed
	})

	for range 2 {
		if err := actor.Run(t.Context()); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	}
	if got := factories.Load(); got != 2 {
		t.Fatalf("expected two server factories, got %d", got)
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
	requirePanic(t, func() { httpserver.NewActor("server", server).SetShutdownTimeout(0) })
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
