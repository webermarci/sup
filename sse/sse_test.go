package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func connectURL(client *http.Client, url string) ConnectFunc {
	return func(ctx context.Context, lastEventID string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/event-stream")
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		return client.Do(req)
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

func TestActorParsesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		io.WriteString(w, ": heartbeat\nevent: update\nid: 101\ndata: hello\ndata: world\n\n")
	}))
	defer server.Close()

	var received Event
	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(event Event) {
		received = event
	})

	err := actor.Run(t.Context())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
	if received.ID != "101" || received.Type != "update" || received.Data != "hello\nworld" {
		t.Fatalf("unexpected event: %+v", received)
	}
}

func TestActorParsesCRLF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\r\n\r\ndata: second\r\n\r\n")
	}))
	defer server.Close()

	var events []Event
	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(event Event) {
		events = append(events, event)
	})

	if err := actor.Run(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
	if len(events) != 2 || events[0].Data != "first" || events[1].Data != "second" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestActorReplacesInvalidUTF8(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte{'d', 'a', 't', 'a', ':', ' ', 0xff, '\n', '\n'})
	}))
	defer server.Close()

	var received Event
	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(event Event) {
		received = event
	})

	if err := actor.Run(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
	if received.Data != "\uFFFD" {
		t.Fatalf("expected replacement character, got %q", received.Data)
	}
}

func TestActorUnexpectedEOFIsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(Event) {})
	if err := actor.Run(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
}

func TestActorNoContentIsCleanStop(t *testing.T) {
	actor := NewActor(t.Name(), func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	}, func(Event) {})

	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("expected clean stop, got %v", err)
	}
}

func TestActorStripsBOMAndClearsLastEventID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "\uFEFFid: first\ndata: one\n\nid:\ndata: two\n\n")
	}))
	defer server.Close()

	events := make([]Event, 0, 2)
	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(event Event) {
		events = append(events, event)
	})

	err := actor.Run(t.Context())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two events, got %d", len(events))
	}
	if events[0].ID != "first" || events[0].Data != "one" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].ID != "" || events[1].Data != "two" {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if actor.LastEventID() != "" {
		t.Fatalf("expected cleared last event ID, got %q", actor.LastEventID())
	}
}

func TestActorCancellationIsClean(t *testing.T) {
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(connected)
		<-r.Context().Done()
	}))
	defer server.Close()

	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(Event) {})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	<-connected
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
}

func TestActorRejectsConcurrentRun(t *testing.T) {
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(connected)
		<-r.Context().Done()
	}))
	defer server.Close()

	actor := NewActor(t.Name(), connectURL(server.Client(), server.URL), func(Event) {})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	<-connected
	if err := actor.Run(t.Context()); !errors.Is(err, ErrActorRunning) {
		t.Fatalf("expected ErrActorRunning, got %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
}

func TestActorValidatesResponse(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		want     string
	}{
		{
			name: "status",
			response: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("")),
			},
			want: "status code",
		},
		{
			name: "content type",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader("{}")),
			},
			want: "content type",
		},
		{
			name: "charset",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=iso-8859-1"}},
				Body:       io.NopCloser(strings.NewReader("")),
			},
			want: "charset",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor := NewActor(t.Name(), func(context.Context, string) (*http.Response, error) {
				return test.response, nil
			}, func(Event) {})
			if err := actor.Run(t.Context()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %s error, got %v", test.want, err)
			}
		})
	}
}

func TestActorPassesLastEventIDToConnect(t *testing.T) {
	var receivedIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "id: msg_99\ndata: hi\n\n")
	}))
	defer server.Close()

	connect := connectURL(server.Client(), server.URL)
	actor := NewActor(t.Name(), func(ctx context.Context, lastEventID string) (*http.Response, error) {
		receivedIDs = append(receivedIDs, lastEventID)
		return connect(ctx, lastEventID)
	}, func(Event) {})

	for range 2 {
		if err := actor.Run(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected unexpected EOF, got %v", err)
		}
	}

	if len(receivedIDs) != 2 || receivedIDs[0] != "" || receivedIDs[1] != "msg_99" {
		t.Fatalf("unexpected last event IDs: %v", receivedIDs)
	}
}

func TestActorReturnsConnectError(t *testing.T) {
	want := errors.New("connect failed")
	actor := NewActor(t.Name(), func(context.Context, string) (*http.Response, error) {
		return nil, want
	}, func(Event) {})

	if err := actor.Run(t.Context()); !errors.Is(err, want) {
		t.Fatalf("expected connection error, got %v", err)
	}
}

func TestActorRejectsNilResponse(t *testing.T) {
	actor := NewActor(t.Name(), func(context.Context, string) (*http.Response, error) {
		return nil, nil
	}, func(Event) {})
	if err := actor.Run(t.Context()); !errors.Is(err, ErrNilResponse) {
		t.Fatalf("expected ErrNilResponse, got %v", err)
	}
}

func TestActorRejectsNilBody(t *testing.T) {
	actor := NewActor(t.Name(), func(context.Context, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		}, nil
	}, func(Event) {})
	if err := actor.Run(t.Context()); !errors.Is(err, ErrNilBody) {
		t.Fatalf("expected ErrNilBody, got %v", err)
	}
}

func TestActorInspect(t *testing.T) {
	actor := NewActor("events", func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	}, func(Event) {})
	if actor.ID() != "events" {
		t.Fatalf("unexpected actor id: %q", actor.ID())
	}
	if spec := actor.Inspect(); spec.Kind != "sse" {
		t.Fatalf("unexpected actor spec: %+v", spec)
	}
}

func TestNewActorValidation(t *testing.T) {
	connect := func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	}
	requirePanic(t, func() { NewActor("", connect, func(Event) {}) })
	requirePanic(t, func() { NewActor("actor", nil, func(Event) {}) })
	requirePanic(t, func() { NewActor("actor", connect, nil) })

	actor := NewActor("actor", connect, func(Event) {})
	requirePanic(t, func() { actor.Run(nil) })
}
