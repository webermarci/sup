package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sync"

	"github.com/webermarci/sup"
)

//go:embed static/*
var staticFS embed.FS

// DashboardOption represents a functional option for configuring the Dashboard.
// It allows adding providers to observe, which will be reflected in the real-time UI.
type DashboardOption func(*Dashboard)

// WithObserve adds a read-only card to the dashboard for the given signal.
// The dashboard will subscribe to the signal for updates and reflect changes in the UI,
// but will not allow user input to update the signal.
func WithObserve[V any](signal sup.ReadableSignal[V]) DashboardOption {
	return func(d *Dashboard) {
		name := signal.Name()

		d.schema = append(d.schema, Card{
			Name: name,
			Type: inferType[V](),
		})

		d.streams = append(d.streams, func(ctx context.Context) error {
			ch := signal.Subscribe(ctx)
			for {
				select {
				case <-ctx.Done():
					return nil
				case val, ok := <-ch:
					if !ok {
						return fmt.Errorf("provider %s closed subscription", name)
					}

					d.mu.Lock()
					d.lastValues[name] = val
					d.mu.Unlock()

					update := map[string]any{"name": name, "value": val}
					if b, err := json.Marshal(update); err == nil {
						d.broadcast("update", b)
					}
				}
			}
		})
	}
}

// Dashboard is an actor that serves a web-based dashboard for monitoring various providers and states.
type Dashboard struct {
	*sup.BaseActor
	schema     []Card
	clients    map[chan []byte]struct{}
	streams    []func(context.Context) error
	lastValues map[string]any
	mu         sync.RWMutex
}

// NewDashboard creates a new Dashboard instance with the given name and options.
func NewDashboard(name string, opts ...DashboardOption) *Dashboard {
	d := &Dashboard{
		BaseActor:  sup.NewBaseActor(name),
		clients:    make(map[chan []byte]struct{}),
		lastValues: make(map[string]any),
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Handler returns an http.Handler that serves the dashboard API and frontend assets.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	tmpl := template.Must(template.ParseFS(staticFS, "static/index.html"))

	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	mux.HandleFunc("GET /{$}", staticHandler(d, tmpl))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS))))
	mux.HandleFunc("GET /api/events", getEvents(d))

	return mux
}

// Run starts all dashboard streams and blocks until the context is canceled or a stream returns an error.
func (d *Dashboard) Run(ctx context.Context) error {
	errCh := make(chan error, len(d.streams))

	for _, stream := range d.streams {
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

func (d *Dashboard) broadcast(eventType string, data []byte) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	msg := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", eventType, data)
	for ch := range d.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}
