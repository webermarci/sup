package hub

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"github.com/webermarci/sup"
)

//go:embed debug/build/index.html
var debugIndex []byte

const defaultHubEventLimit = 128

// Option configures a Hub.
type Option func(*Hub)

// WithEventLimit sets the maximum number of events stored per source.
func WithEventLimit(limit int) Option {
	if limit < 0 {
		panic("sup: event limit must be non-negative")
	}

	return func(h *Hub) {
		h.eventLimit = limit
	}
}

// WithActor registers an actor with the hub.
func WithActor(actor sup.Actor) Option {
	if actor == nil {
		panic("sup: cannot register nil actor")
	}

	return func(h *Hub) {
		h.RegisterActor(actor)
	}
}

// WithSignal registers a readable signal with the hub.
func WithSignal[V any](signal sup.ReadableSignal[V]) Option {
	if signal == nil {
		panic("sup: cannot register nil signal")
	}

	if signal.ID() == "" {
		panic("sup: signal id cannot be empty")
	}

	return func(h *Hub) {
		id := signal.ID()
		h.registerSignal(hubSignal{
			ID:    id,
			Spec:  signal.Inspect(),
			Type:  inferSignalValueType[V](),
			Value: signal.Read(),
		}, func(ctx context.Context) error {
			values := signal.Subscribe(ctx)

			for {
				select {
				case <-ctx.Done():
					return nil

				case value, ok := <-values:
					if !ok {
						return nil
					}

					h.updateSignalValue(id, value)
					h.publish(sup.NewEvent(sup.EventSignalUpdated, id, sup.SignalUpdatedPayload{
						Value: value,
					}))
				}
			}
		})
	}
}

// Hub exposes actors, controls, signals, and events over HTTP.
type Hub struct {
	*sup.BaseActor
	actors     map[string]sup.Actor
	signals    map[string]hubSignal
	streams    []func(context.Context) error
	events     map[string][]sup.Event
	clients    map[chan []byte]struct{}
	eventLimit int
	mu         sync.RWMutex
}

// New creates a hub with the given id and options.
func New(id string, options ...Option) *Hub {
	hub := &Hub{
		BaseActor:  sup.NewBaseActor(id),
		actors:     make(map[string]sup.Actor),
		signals:    make(map[string]hubSignal),
		events:     make(map[string][]sup.Event),
		clients:    make(map[chan []byte]struct{}),
		eventLimit: defaultHubEventLimit,
	}

	for _, option := range options {
		option(hub)
	}

	return hub
}

// RegisterActor registers an actor with the hub.
func (h *Hub) RegisterActor(actor sup.Actor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.actors[actor.ID()] = actor
}

func (h *Hub) registerSignal(signal hubSignal, stream func(context.Context) error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.signals[signal.ID]; exists {
		panic("sup: duplicate signal id: " + signal.ID)
	}

	h.signals[signal.ID] = signal
	h.streams = append(h.streams, stream)
}

// Handler returns the HTTP handler for the hub API and debug UI.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /actors", h.handleActors)
	mux.HandleFunc("GET /actors/{actorID}", h.handleActor)
	mux.HandleFunc("GET /actors/{actorID}/controls", h.handleControls)
	mux.HandleFunc("POST /actors/{actorID}/controls/{controlName}", h.handleControl)

	mux.HandleFunc("GET /signals", h.handleSignals)
	mux.HandleFunc("GET /signals/{signalID}", h.handleSignal)

	mux.HandleFunc("GET /events", h.handleEvents)
	mux.HandleFunc("GET /events/stream", h.handleEventStream)

	mux.HandleFunc("GET /debug", h.serveDebug)
	mux.HandleFunc("GET /debug/{$}", h.serveDebug)

	return mux
}

// Observer returns a supervisor observer that records actor lifecycle events.
func (h *Hub) Observer() *sup.SupervisorObserver {
	return &sup.SupervisorObserver{
		OnActorRegistered: func(supervisor *sup.Supervisor, actor sup.Actor) {
			h.RegisterActor(actor)
			h.publish(sup.NewEvent(sup.EventActorRegistered, actor.ID(), sup.ActorRegisteredPayload{
				SupervisorID: supervisor.ID(),
			}))
		},
		OnActorStarted: func(supervisor *sup.Supervisor, actor sup.Actor) {
			h.publish(sup.NewEvent(sup.EventActorStarted, actor.ID(), sup.ActorStartedPayload{
				SupervisorID: supervisor.ID(),
			}))
		},
		OnActorStopped: func(supervisor *sup.Supervisor, actor sup.Actor, err error) {
			h.publish(sup.NewEvent(sup.EventActorStopped, actor.ID(), sup.ActorStoppedPayload{
				SupervisorID: supervisor.ID(),
				Error:        hubErrorString(err),
			}))
		},
		OnActorRestarting: func(supervisor *sup.Supervisor, actor sup.Actor, restartCount int, lastErr error) {
			h.publish(sup.NewEvent(sup.EventActorRestarting, actor.ID(), sup.ActorRestartingPayload{
				SupervisorID: supervisor.ID(),
				RestartCount: restartCount,
				LastError:    hubErrorString(lastErr),
			}))
		},
		OnSupervisorTerminal: func(supervisor *sup.Supervisor, err error) {
			h.publish(sup.NewEvent(sup.EventSupervisorTerminal, supervisor.ID(), sup.SupervisorTerminalPayload{
				Error: hubErrorString(err),
			}))
		},
	}
}

// Inspect returns the hub spec.
func (h *Hub) Inspect() sup.Spec {
	h.mu.RLock()
	actorCount := len(h.actors)
	signalCount := len(h.signals)
	h.mu.RUnlock()

	return sup.Spec{
		Kind:         "hub",
		Dependencies: []string{},
		Metadata: map[string]string{
			"actor_count":  strconv.Itoa(actorCount),
			"signal_count": strconv.Itoa(signalCount),
		},
	}
}

// Run starts registered signal streams until they stop, fail, or the context is canceled.
func (h *Hub) Run(ctx context.Context) error {
	streams := h.streamsSnapshot()
	if len(streams) == 0 {
		<-ctx.Done()
		return nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(streams))
	for _, stream := range streams {
		go func(stream func(context.Context) error) {
			errCh <- stream(streamCtx)
		}(stream)
	}

	remaining := len(streams)
	waitForStreams := func() {
		for remaining > 0 {
			<-errCh
			remaining--
		}
	}

	for remaining > 0 {
		select {
		case <-ctx.Done():
			cancel()
			waitForStreams()
			return nil

		case err := <-errCh:
			remaining--
			if err != nil {
				cancel()
				waitForStreams()
				return err
			}
		}
	}

	<-ctx.Done()
	return nil
}

func (h *Hub) actor(id string) (sup.Actor, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	actor, ok := h.actors[id]
	return actor, ok
}

func (h *Hub) streamsSnapshot() []func(context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	streams := make([]func(context.Context) error, len(h.streams))
	copy(streams, h.streams)
	return streams
}

func (h *Hub) actorsSnapshot() []sup.Actor {
	h.mu.RLock()
	defer h.mu.RUnlock()

	actors := make([]sup.Actor, 0, len(h.actors))
	for _, actor := range h.actors {
		actors = append(actors, actor)
	}

	sort.Slice(actors, func(i, j int) bool {
		return actors[i].ID() < actors[j].ID()
	})

	return actors
}

func (h *Hub) eventsSnapshot() map[string][]sup.Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := make(map[string][]sup.Event)
	maps.Copy(events, h.events)

	return events
}

func (h *Hub) publish(event sup.Event) {
	msg, err := formatHubEventSSE(event)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	key := event.SourceID
	h.events[key] = append(h.events[key], event)
	if h.eventLimit > 0 && len(h.events[key]) > h.eventLimit {
		h.events[key] = h.events[key][len(h.events[key])-h.eventLimit:]
	}

	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func formatHubEventSSE(event sup.Event) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "id: %s\n", event.ID)
	fmt.Fprintf(&buf, "event: %s\n", event.Type)
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	return buf.Bytes(), nil
}

func hubErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
