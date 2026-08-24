package hub

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/rx"
)

const defaultEventHistoryLimit = 128

// Hub exposes explicitly registered actors and signals over HTTP, records a
// bounded recent event history, and projects runtime registration events into
// a supervision graph.
type Hub struct {
	id                string
	handler           http.Handler
	exposedActors     map[string]struct{}
	signals           map[string]registeredSignal
	graphNodes        map[string]graphNode
	parents           map[string]string
	events            []hubEvent
	eventHistoryLimit int
	clients           map[chan []byte]struct{}
	mu                sync.RWMutex
}

// New creates a hub with the given id. Configure the hub before it is run.
func New(id string) *Hub {
	if id == "" {
		panic("hub: hub id cannot be empty")
	}

	h := &Hub{
		id:                id,
		exposedActors:     make(map[string]struct{}),
		signals:           make(map[string]registeredSignal),
		graphNodes:        make(map[string]graphNode),
		parents:           make(map[string]string),
		clients:           make(map[chan []byte]struct{}),
		eventHistoryLimit: defaultEventHistoryLimit,
	}
	h.handler = h.newHandler()
	return h
}

// SetEventHistoryLimit sets the maximum number of recent events retained
// across all registered sources and returns the hub for chaining. A zero limit
// disables event history without disabling the live event stream.
func (h *Hub) SetEventHistoryLimit(limit int) *Hub {
	if limit < 0 {
		panic("hub: event history limit must be non-negative")
	}
	h.eventHistoryLimit = limit
	return h
}

// RegisterActors exposes actors through the hub and returns the hub for chaining.
// Runtime events for actors that are not explicitly registered remain private.
func (h *Hub) RegisterActors(actors ...sup.Actor) *Hub {
	actorIDs := make(map[string]struct{}, len(h.exposedActors)+len(actors))
	for id := range h.exposedActors {
		actorIDs[id] = struct{}{}
	}

	for _, actor := range actors {
		id := actor.ID()
		if id == "" {
			panic("hub: actor id cannot be empty")
		}
		if _, exists := actorIDs[id]; exists {
			panic("hub: duplicate actor id: " + id)
		}
		if _, exists := h.signals[id]; exists {
			panic("hub: duplicate source id: " + id)
		}
		actorIDs[id] = struct{}{}
	}

	for _, actor := range actors {
		id := actor.ID()
		h.exposedActors[id] = struct{}{}
		h.graphNodes[id] = newGraphNode(actor)
	}
	return h
}

// RegisterSignal exposes a readable signal through the hub and returns the hub for
// chaining.
func (h *Hub) RegisterSignal[V any](signal rx.Readable[V]) *Hub {
	h.registerSignal(newReadableSignal(signal))
	return h
}

// RegisterWritableSignal exposes a writable signal through the hub and returns the
// hub for chaining. Its value can be changed with PATCH /signals/{signalID}.
func (h *Hub) RegisterWritableSignal[V any](signal rx.Writable[V]) *Hub {
	h.registerSignal(newWritableSignal(signal))
	return h
}

// ID returns the hub id.
func (h *Hub) ID() string {
	return h.id
}

func (h *Hub) registerSignal(signal registeredSignal) {
	if signal.id == "" {
		panic("hub: signal id cannot be empty")
	}
	if _, exists := h.signals[signal.id]; exists {
		panic("hub: duplicate signal id: " + signal.id)
	}
	if _, exists := h.exposedActors[signal.id]; exists {
		panic("hub: duplicate source id: " + signal.id)
	}
	h.signals[signal.id] = signal
}

// ServeHTTP serves the hub debug page and API.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func (h *Hub) newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveDebugPage)

	mux.HandleFunc("GET /signals", h.handleSignals)
	mux.HandleFunc("GET /signals/{signalID}", h.handleSignal)
	mux.Handle(
		"PATCH /signals/{signalID}",
		http.NewCrossOriginProtection().Handler(http.HandlerFunc(h.handleSetSignal)),
	)

	mux.HandleFunc("GET /graph", h.handleGraph)
	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("GET /events/stream", h.handleEventStream)

	return mux
}

// HandleEvent projects a runtime event emitted by a supervisor tree. All
// registration events contribute structural supervisor information, while
// only explicitly exposed actors contribute public lifecycle events.
func (h *Hub) HandleEvent(event sup.Event) {
	h.projectRuntimeEvent(event)

	_, exposed := h.exposedActors[event.Actor.ID()]
	if !exposed {
		return
	}

	h.publish(hubEventFromRuntime(event))
}

// Run observes registered signals and publishes their changes until ctx is
// canceled. Signal values themselves are always read directly from the source.
func (h *Hub) Run(ctx context.Context) error {
	var streams sync.WaitGroup
	defer streams.Wait()

	for _, signal := range h.signals {
		streams.Go(func() {
			signal.observe(ctx, func(value any) {
				h.publish(newHubEvent(
					eventSignalUpdated,
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

func (h *Hub) eventsSnapshot() []hubEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return slices.Clone(h.events)
}

func (h *Hub) publish(event hubEvent) {
	snapshot, data, err := snapshotHubEvent(event)
	if err != nil {
		return
	}

	msg := fmt.Appendf(nil, "id: %s\nevent: %s\ndata: %s\n\n", snapshot.ID, snapshot.Type, data)

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
	w.Header().Set("Cache-Control", "no-store")
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
