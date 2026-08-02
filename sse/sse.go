// Package sse adapts HTTP server-sent event streams to sup actors.
package sse

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/webermarci/sup"
)

var (
	// ErrActorRunning is returned when Run is called while the actor is already
	// consuming or establishing a stream.
	ErrActorRunning = errors.New("sse: actor is already running")

	// ErrNilResponse is returned when a ConnectFunc returns a nil response
	// without an error.
	ErrNilResponse = errors.New("sse: connect function returned nil response")

	// ErrNilBody is returned when a successful response has no body.
	ErrNilBody = errors.New("sse: response body is nil")
)

const maxLineSize = 1024 * 1024

// Event is one server-sent event.
type Event struct {
	ID   string
	Type string
	Data string
}

// ConnectFunc establishes one SSE connection. lastEventID is the most recent
// valid id field parsed by the actor and may be sent as Last-Event-ID. The
// function owns request construction and HTTP client configuration; the actor
// closes the body of a response returned without an error.
type ConnectFunc func(ctx context.Context, lastEventID string) (*http.Response, error)

// Actor establishes one connection per Run call and parses its event stream.
// Reconnection policy belongs to the supervisor.
type Actor struct {
	id      string
	connect ConnectFunc
	handle  func(Event)
	lastID  string
	mu      sync.RWMutex
	running atomic.Bool
}

var _ sup.Actor = (*Actor)(nil)
var _ sup.Inspectable = (*Actor)(nil)

// NewActor creates an SSE actor with the given connection and event handlers.
// handle is called synchronously while consuming the stream.
func NewActor(id string, connect ConnectFunc, handle func(Event)) *Actor {
	if id == "" {
		panic("sse: actor id cannot be empty")
	}

	if connect == nil {
		panic("sse: connect function cannot be nil")
	}

	if handle == nil {
		panic("sse: event handler cannot be nil")
	}

	return &Actor{id: id, connect: connect, handle: handle}
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Inspect returns the SSE actor spec.
func (a *Actor) Inspect() sup.Spec {
	return sup.Spec{Kind: "sse"}
}

// LastEventID returns the value of the most recently parsed valid id field.
func (a *Actor) LastEventID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastID
}

// Run establishes and consumes one SSE connection. Context cancellation and
// HTTP 204 are clean actor shutdowns; other connection and stream failures are
// returned to the supervisor.
func (a *Actor) Run(ctx context.Context) (err error) {
	if ctx == nil {
		panic("sse: context cannot be nil")
	}

	if !a.running.CompareAndSwap(false, true) {
		return ErrActorRunning
	}
	defer a.running.Store(false)

	defer func() {
		if ctx.Err() != nil {
			err = nil
		}
	}()

	response, err := a.connect(ctx, a.LastEventID())
	if err != nil {
		return err
	}

	if response == nil {
		return ErrNilResponse
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	// The SSE protocol uses 204 to tell the client to stop reconnecting. A nil
	// result lets a Transient supervisor stop while Permanent can still restart.
	if response.StatusCode == http.StatusNoContent {
		return nil
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("sse: unexpected status code: %d", response.StatusCode)
	}

	if response.Body == nil {
		return ErrNilBody
	}

	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/event-stream") {
		return fmt.Errorf("sse: unexpected content type: %q", response.Header.Get("Content-Type"))
	}

	if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		return fmt.Errorf("sse: unexpected event-stream charset: %q", charset)
	}

	return a.consume(ctx, response.Body)
}

func (a *Actor) consume(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	var data bytes.Buffer
	event := Event{ID: a.LastEventID()}
	firstLine := true

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}

		line := scanner.Text()
		line = strings.ToValidUTF8(line, "\uFFFD")
		if firstLine {
			line = strings.TrimPrefix(line, "\uFEFF")
			firstLine = false
		}

		if line == "" {
			if data.Len() > 0 {
				value := data.String()
				value = strings.TrimSuffix(value, "\n")
				event.Data = value
				a.handle(event)
				data.Reset()
			}
			event = Event{ID: a.LastEventID()}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if found {
			value = strings.TrimPrefix(value, " ")
		}

		switch field {
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
		case "event":
			event.Type = value
		case "id":
			if !strings.ContainsRune(value, 0) {
				a.mu.Lock()
				a.lastID = value
				a.mu.Unlock()
				event.ID = value
			}
		case "retry":
			// Reconnection timing belongs to the supervisor.
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse: read stream: %w", err)
	}

	if ctx.Err() != nil {
		return nil
	}

	return fmt.Errorf("sse: stream closed: %w", io.ErrUnexpectedEOF)
}
