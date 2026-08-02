package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/rx"
)

const defaultEventHistoryLimit = 128

// ErrHubRunning is returned when Run is called while the hub is already
// running.
var ErrHubRunning = errors.New("hub: hub is already running")

// Option configures a Hub during construction.
type Option func(*Hub)

// WithEventHistoryLimit sets the maximum number of recent events retained
// across all registered sources. A zero limit disables event history without
// disabling the live event stream.
func WithEventHistoryLimit(limit int) Option {
	if limit < 0 {
		panic("hub: event history limit must be non-negative")
	}

	return func(h *Hub) {
		h.eventHistoryLimit = limit
	}
}

// WithActor exposes an actor through the hub. Runtime events for actors that
// are not explicitly registered remain private.
func WithActor(actor sup.Actor) Option {
	if actor == nil {
		panic("hub: cannot register nil actor")
	}

	return func(h *Hub) {
		id := actor.ID()
		if id == "" {
			panic("hub: actor id cannot be empty")
		}
		if _, exists := h.actors[id]; exists {
			panic("hub: duplicate actor id: " + id)
		}
		if _, exists := h.signals[id]; exists {
			panic("hub: duplicate source id: " + id)
		}

		h.actors[id] = actor
	}
}

// WithSignal exposes a readable signal through the hub.
func WithSignal[V any](signal rx.Readable[V]) Option {
	if signal == nil {
		panic("hub: cannot register nil signal")
	}

	return func(h *Hub) {
		h.addSignal(newReadableSignal(signal))
	}
}

// WithWritableSignal exposes a writable signal through the hub. Its value can
// be changed with PATCH /signals/{signalID}.
func WithWritableSignal[V any](signal rx.Writable[V]) Option {
	if signal == nil {
		panic("hub: cannot register nil writable signal")
	}

	return func(h *Hub) {
		h.addSignal(newWritableSignal(signal))
	}
}

// Hub exposes explicitly registered actors and signals over HTTP, records a
// bounded recent event history, and projects runtime registration events into
// a supervision graph.
type Hub struct {
	id string

	// actors and signals are immutable after New returns.
	actors            map[string]sup.Actor
	signals           map[string]registeredSignal
	graphNodes        map[string]graphNode
	parents           map[string]string
	events            []hubEvent
	eventHistoryLimit int
	clients           map[chan []byte]struct{}
	mu                sync.RWMutex
	running           atomic.Bool
}

// New creates a hub with the given id and options.
func New(id string, options ...Option) *Hub {
	if id == "" {
		panic("hub: hub id cannot be empty")
	}

	h := &Hub{
		id:                id,
		actors:            make(map[string]sup.Actor),
		signals:           make(map[string]registeredSignal),
		graphNodes:        make(map[string]graphNode),
		parents:           make(map[string]string),
		clients:           make(map[chan []byte]struct{}),
		eventHistoryLimit: defaultEventHistoryLimit,
	}

	for _, option := range options {
		if option == nil {
			panic("hub: option cannot be nil")
		}
		option(h)
	}

	for id, actor := range h.actors {
		h.graphNodes[id] = newGraphNode(actor)
	}

	return h
}

// ID returns the hub id.
func (h *Hub) ID() string {
	return h.id
}

func (h *Hub) addSignal(signal registeredSignal) {
	if signal.id == "" {
		panic("hub: signal id cannot be empty")
	}

	if _, exists := h.signals[signal.id]; exists {
		panic("hub: duplicate signal id: " + signal.id)
	}

	if _, exists := h.actors[signal.id]; exists {
		panic("hub: duplicate source id: " + signal.id)
	}

	h.signals[signal.id] = signal
}

// Handler returns the HTTP handler for the hub debug page and API.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveDebugPage)

	mux.HandleFunc("GET /actors", h.handleActors)
	mux.HandleFunc("GET /actors/{actorID}", h.handleActor)

	mux.HandleFunc("GET /signals", h.handleSignals)
	mux.HandleFunc("GET /signals/{signalID}", h.handleSignal)
	mux.HandleFunc("PATCH /signals/{signalID}", h.handleSetSignal)

	mux.HandleFunc("GET /graph", h.handleGraph)
	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("GET /events/stream", h.handleEventStream)

	return mux
}

// HandleEvent projects a runtime event emitted by a supervisor tree. All
// registration events contribute structural supervisor information, while
// only explicitly exposed actors contribute public lifecycle events.
func (h *Hub) HandleEvent(event sup.Event) {
	if event.Actor == nil {
		return
	}

	h.projectRuntimeEvent(event)

	_, exposed := h.actors[event.Actor.ID()]
	if !exposed {
		return
	}

	h.publish(hubEventFromRuntime(event))
}

// Inspect returns the hub spec.
func (h *Hub) Inspect() sup.Spec {
	dependencies := slices.Sorted(maps.Keys(h.signals))

	return sup.Spec{
		Kind:         "hub",
		Dependencies: dependencies,
		Metadata:     map[string]any{},
	}
}

// Run observes registered signals and publishes their changes until ctx is
// canceled. Signal values themselves are always read directly from the source.
func (h *Hub) Run(ctx context.Context) error {
	if ctx == nil {
		panic("hub: context cannot be nil")
	}

	if !h.running.CompareAndSwap(false, true) {
		return ErrHubRunning
	}
	defer h.running.Store(false)

	var streams sync.WaitGroup
	defer streams.Wait()

	for _, signal := range h.signals {
		streams.Go(func() {
			signal.observe(ctx, func(value any) {
				h.publish(newHubEvent(
					EventSignalUpdated,
					signal.id,
					signalUpdatedPayload{Value: value},
					time.Time{},
				))
			})
		})
	}

	<-ctx.Done()
	return nil
}

func (h *Hub) actorsSnapshot() []sup.Actor {
	return slices.SortedFunc(maps.Values(h.actors), func(a, b sup.Actor) int {
		return cmp.Compare(a.ID(), b.ID())
	})
}

func (h *Hub) eventsSnapshot() []hubEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return slices.Clone(h.events)
}

func (h *Hub) publish(event hubEvent) {
	snapshot, err := snapshotHubEvent(event)
	if err != nil {
		return
	}

	msg, err := formatHubEventSSE(snapshot)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.eventHistoryLimit > 0 {
		h.events = append(h.events, snapshot)
		if len(h.events) > h.eventHistoryLimit {
			h.events = slices.Clone(h.events[len(h.events)-h.eventHistoryLimit:])
		}
	}

	for client := range h.clients {
		select {
		case client <- msg:
		default:
		}
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
