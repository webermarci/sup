package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/webermarci/sup"
)

type RegistryOption func(*Registry)

func WithActor(actor sup.Actor, opts ...ActorOption) RegistryOption {
	return func(r *Registry) {
		id := actor.ID()
		if _, exists := r.actorsByID[id]; exists {
			panic("sup: duplicate exposed actor id: " + id)
		}

		ea := &ExposedActor{
			ID:   actor.ID(),
			Spec: actor.Inspect(),
		}

		for _, opt := range opts {
			opt(ea)
		}

		r.actorsByID[id] = ea
		r.actors = append(r.actors, ea)
	}
}

func WithSignal[V any](signal sup.ReadableSignal[V]) RegistryOption {
	return func(r *Registry) {
		id := signal.ID()
		if _, exists := r.signalsByID[id]; exists {
			panic("sup: duplicate exposed signal id: " + id)
		}

		es := &ExposedSignal{
			ID:    id,
			Spec:  signal.Inspect(),
			Value: signal.Read(),
		}

		r.signalsByID[id] = es
		r.signals = append(r.signals, es)

		r.streams = append(r.streams, func(ctx context.Context) error {
			ch := signal.Subscribe(ctx)

			for {
				select {
				case <-ctx.Done():
					return nil

				case value, ok := <-ch:
					if !ok {
						return nil
					}

					r.mu.Lock()
					es.Value = value
					r.mu.Unlock()

					r.broadcast("signal:update", map[string]any{
						"timestamp": time.Now().UnixMilli(),
						"id":        id,
						"value":     value,
					})
				}
			}
		})
	}
}

type Registry struct {
	*sup.BaseActor
	actorsByID  map[string]*ExposedActor
	actors      []*ExposedActor
	signalsByID map[string]*ExposedSignal
	signals     []*ExposedSignal
	streams     []func(context.Context) error
	clients     map[chan []byte]struct{}
	mu          sync.RWMutex
}

func NewRegistry(id string, opts ...RegistryOption) *Registry {
	r := &Registry{
		BaseActor:   sup.NewBaseActor(id),
		actorsByID:  make(map[string]*ExposedActor),
		signalsByID: make(map[string]*ExposedSignal),
		clients:     make(map[chan []byte]struct{}),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *Registry) Run(ctx context.Context) error {
	errCh := make(chan error, len(r.streams))

	for _, stream := range r.streams {
		go func(s func(context.Context) error) {
			if err := s(ctx); err != nil {
				errCh <- err
			}
		}(stream)
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (r *Registry) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", r.handleIndex)
	mux.HandleFunc("GET /events", r.handleEvents)
	mux.HandleFunc("POST /actors/{actorID}/casts/{castName}", r.handleCast)
	mux.HandleFunc("POST /actors/{actorID}/calls/{callName}", r.handleCall)

	return mux
}

func (r *Registry) broadcast(eventType string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	buf.WriteString("event: ")
	buf.WriteString(eventType)
	buf.WriteString("\n")

	s := string(data)
	s = strings.ReplaceAll(s, "\n", "\ndata: ")

	buf.WriteString("data: ")
	buf.WriteString(s)
	buf.WriteString("\n\n")

	msg := buf.Bytes()

	r.mu.RLock()
	defer r.mu.RUnlock()

	for ch := range r.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (r *Registry) handleIndex(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]any{
		"actors":  r.actors,
		"signals": r.signals,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *Registry) handleEvents(w http.ResponseWriter, req *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 16)

	r.mu.Lock()
	r.clients[ch] = struct{}{}
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.clients, ch)
		r.mu.Unlock()
		close(ch)
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-req.Context().Done():
			return

		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()

		case msg := <-ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (r *Registry) handleCast(w http.ResponseWriter, req *http.Request) {
	actorID := req.PathValue("actorID")
	castName := req.PathValue("castName")

	ea, ok := r.actorsByID[actorID]
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	if ea.Control == nil {
		http.Error(w, "actor has no control", http.StatusNotFound)
		return
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(raw) == 0 {
		raw = []byte("{}")
	}

	if err := ea.Control.Cast(req.Context(), castName, raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]any{"ok": true}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *Registry) handleCall(w http.ResponseWriter, req *http.Request) {
	actorID := req.PathValue("actorID")
	callName := req.PathValue("callName")

	ea, ok := r.actorsByID[actorID]
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	if ea.Control == nil {
		http.Error(w, "actor has no control", http.StatusNotFound)
		return
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(raw) == 0 {
		raw = []byte("{}")
	}

	res, err := ea.Control.Call(req.Context(), callName, raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
