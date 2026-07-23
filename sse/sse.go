package sse

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
)

// Event represents a single SSE event with its ID, data, and optional name.
type Event struct {
	ID   string
	Data string
	Name string
}

// ActorOption defines a function type for configuring an Actor.
type ActorOption func(*Actor)

// WithTimeout sets the duration after which the SSE connection will be considered timed out if no events are received.
func WithTimeout(d time.Duration) ActorOption {
	return func(a *Actor) {
		a.timeout = d
	}
}

// WithLastEventID sets the initial Last-Event-ID to be used when connecting.
func WithLastEventID(id string) ActorOption {
	return func(a *Actor) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.lastID = id
	}
}

// WithOnConnect allows the caller to provide a callback function that will be invoked when the Actor successfully connects to the SSE endpoint.
func WithOnConnect(handler func(url string, lastID string)) ActorOption {
	return func(a *Actor) {
		a.onConnect = handler
	}
}

// WithOnError allows the caller to provide a callback function that will be invoked whenever an error occurs during the Actor's operation.
func WithOnError(handler func(error)) ActorOption {
	return func(a *Actor) {
		a.onError = handler
	}
}

// WithHTTPClient allows the caller to provide a custom http.Client for making requests to the SSE endpoint.
func WithHTTPClient(c *http.Client) ActorOption {
	return func(a *Actor) {
		a.client = c
	}
}

// Actor is responsible for connecting to an SSE endpoint, reading and parsing incoming events, and invoking a handler function for each event.
type Actor struct {
	id        string
	url       string
	timeout   time.Duration
	client    *http.Client
	lastID    string
	onConnect func(url string, lastID string)
	onEvent   func(Event)
	onError   func(error)
	mu        sync.RWMutex
}

// NewActor creates a new Actor with the specified URL, event handler, and optional configuration options.
func NewActor(id string, url string, onEvent func(Event), opts ...ActorOption) *Actor {
	a := &Actor{
		id:      id,
		url:     url,
		onEvent: onEvent,
		timeout: 30 * time.Second,
		client: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Inspect returns the SSE actor spec.
func (a *Actor) Inspect() sup.Spec {
	return sup.Spec{Kind: "sse"}
}

// LastEventID returns the ID of the last successfully received event.
func (a *Actor) LastEventID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastID
}

// Run establishes a connection to the SSE endpoint and processes incoming events until the context is canceled or an error occurs.
func (a *Actor) Run(ctx context.Context) (err error) {
	defer func() {
		if ctx.Err() != nil {
			err = nil
		}
		if err != nil && a.onError != nil {
			a.onError(err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
	if err != nil {
		return err
	}

	lastID := a.LastEventID()

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}

	client := a.client
	if client == nil {
		client = &http.Client{}
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	if a.onConnect != nil {
		a.onConnect(a.url, lastID)
	}

	scanner := bufio.NewScanner(res.Body)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxCapacity)

	var timedOut atomic.Bool
	var buf bytes.Buffer
	currentEvent := Event{ID: lastID}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		timedOut.Store(false)
		timer := time.AfterFunc(a.timeout, func() {
			timedOut.Store(true)
			_ = res.Body.Close()
		})

		ok := scanner.Scan()
		timer.Stop()

		if ok {
			line := scanner.Text()

			if line == "" {
				if buf.Len() > 0 {
					data := buf.String()
					if data[len(data)-1] == '\n' {
						data = data[:len(data)-1]
					}
					currentEvent.Data = data
					a.onEvent(currentEvent)
					buf.Reset()
				}
				currentEvent = Event{ID: a.LastEventID()}
				continue
			}

			if strings.HasPrefix(line, ":") {
				continue
			}

			key, value, found := strings.Cut(line, ":")
			if found {
				value = strings.TrimPrefix(value, " ")
			}

			switch key {
			case "data":
				buf.WriteString(value)
				buf.WriteByte('\n')
			case "event":
				currentEvent.Name = value
			case "id":
				if !strings.ContainsRune(value, 0) {
					a.mu.Lock()
					a.lastID = value
					a.mu.Unlock()
					currentEvent.ID = value
				}
			case "retry":
				// Reconnection time is managed by the supervisor, but we parse it for completeness
			}
			continue
		}

		if err := scanner.Err(); err != nil {
			if timedOut.Load() {
				return fmt.Errorf("sse stream timed out after %v", a.timeout)
			}
			return err
		}

		return nil
	}
}
