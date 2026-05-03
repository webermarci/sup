package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/bus"
	"github.com/webermarci/sup/ui/static"
)

type TemplateData struct {
	Cards      []Card
	LastValues map[string]string
}

// State represents a value that can be observed for changes and updated by writing to it. It combines the bus.Provider and bus.Writer interfaces, allowing the dashboard to both subscribe to updates and send commands to update the state.
type State[V any] interface {
	bus.Provider[V]
	bus.Writer[V]
}

// DashboardOption represents a functional option for configuring the Dashboard. It allows adding providers to observe or states to control, which will be reflected in the dashboard UI and API.
type DashboardOption func(*Dashboard)

// WithObserve adds a read-only card to the dashboard for the given provider. The dashboard will subscribe to the provider for updates and reflect changes in the UI, but will not allow user input to update the provider.
func WithObserve[V any](provider bus.Provider[V]) DashboardOption {
	return func(d *Dashboard) {
		name := provider.Name()
		d.schema = append(d.schema, Card{
			Name: name,
			Mode: CardModeRead,
			Type: inferType[V](),
		})

		d.streams = append(d.streams, func(ctx context.Context) error {
			ch := provider.Subscribe(ctx)
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

// WithControl adds a control card to the dashboard for the given state. The dashboard will send command events to the state whenever the user updates the control in the UI, and will also subscribe to the state for updates to reflect changes made outside the dashboard.
func WithControl[V any](state State[V]) DashboardOption {
	return func(d *Dashboard) {
		name := state.Name()
		d.schema = append(d.schema, Card{
			Name: name,
			Mode: CardModeWrite,
			Type: inferType[V](),
		})

		d.commands[name] = func(ctx context.Context, payload []byte) error {
			var val V
			if err := json.Unmarshal(payload, &val); err != nil {
				return err
			}
			return state.Write(ctx, val)
		}

		d.streams = append(d.streams, func(ctx context.Context) error {
			ch := state.Subscribe(ctx)
			for {
				select {
				case <-ctx.Done():
					return nil
				case val, ok := <-ch:
					if !ok {
						return fmt.Errorf("state %s closed subscription", name)
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

// Dashboard represents a real-time UI dashboard that can observe providers and control state through commands. It implements the sup.Actor interface and provides an HTTP handler for serving the dashboard frontend and API.
type Dashboard struct {
	*sup.BaseActor
	schema     []Card
	clients    map[chan []byte]struct{}
	commands   map[string]func(context.Context, []byte) error
	streams    []func(context.Context) error
	lastValues map[string]any
	mu         sync.RWMutex
}

// NewDashboard creates a new Dashboard instance with the given name and options.
func NewDashboard(name string, opts ...DashboardOption) *Dashboard {
	d := &Dashboard{
		BaseActor:  sup.NewBaseActor(name),
		clients:    make(map[chan []byte]struct{}),
		commands:   make(map[string]func(context.Context, []byte) error),
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

	tmpl := template.Must(template.ParseFS(static.FS, "index.html"))

	mux.HandleFunc("OPTIONS /api/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /{$}", staticHandler(d, tmpl))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))
	mux.HandleFunc("GET /api/events", getEvents(d))

	return cors()(mux)
}

// Run starts all dashboard streams and blocks until the context is canceled or a stream returns an error.
func (d *Dashboard) Run(ctx context.Context) error {
	d.Logger().Info("dashboard streams starting", "stream_count", len(d.streams))

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
		d.Logger().Info("dashboard streams shutting down")
		return nil
	case err := <-errCh:
		d.Logger().Error("dashboard stream failed", "err", err)
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
