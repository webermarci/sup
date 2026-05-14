package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/ui/frontend"
)

// DashboardOption represents a functional option for configuring the Dashboard.
// It allows adding providers to observe, which will be reflected in the real-time UI.
type DashboardOption func(*Dashboard)

// WithObserve adds a read-only row to the dashboard for the given signal.
// The dashboard will subscribe to the signal for updates and reflect changes in the UI,
// but will not allow user input to update the signal.
func WithObserve[V any](signal sup.ReadableSignal[V]) DashboardOption {
	return func(d *Dashboard) {
		name := signal.Name()

		index := len(d.nodes)
		d.nodes = append(d.nodes, Node{
			Name:  name,
			Spec:  signal.Inspect(),
			Type:  inferType[V](),
			Value: signal.Read(),
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
					d.nodes[index].Value = val
					d.mu.Unlock()

					update := map[string]any{"timestamp": time.Now().UnixMilli(), "name": name, "value": val}
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
	nodes   []Node
	clients map[chan []byte]struct{}
	streams []func(context.Context) error
	mu      sync.RWMutex
}

// NewDashboard creates a new Dashboard instance with the given name and options.
func NewDashboard(name string, opts ...DashboardOption) *Dashboard {
	d := &Dashboard{
		BaseActor: sup.NewBaseActor(name),
		clients:   make(map[chan []byte]struct{}),
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Handler returns an http.Handler that serves the dashboard API and frontend assets.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/nodes", getNodes(d))
	mux.HandleFunc("GET /api/events", getEvents(d))
	mux.Handle("/", svelteKitHandler(frontend.FileSystem(), "/"))

	return cors()(mux)
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

// Inspect returns the specification.
func (d *Dashboard) Inspect() sup.Spec {
	return sup.Spec{
		Kind:         "dashboard",
		Dependencies: []string{},
		Metadata: map[string]string{
			"observed_count": fmt.Sprintf("%d", len(d.streams)),
		},
	}
}

func (d *Dashboard) broadcast(eventType string, data []byte) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var buf bytes.Buffer
	buf.WriteString("event: ")
	buf.WriteString(eventType)
	buf.WriteString("\n")

	// Ensure multiline JSON is sent as multiple data: lines per the SSE spec.
	s := string(data)
	s = strings.ReplaceAll(s, "\n", "\ndata: ")
	buf.WriteString("data: ")
	buf.WriteString(s)
	buf.WriteString("\n\n")

	msg := buf.Bytes()

	for ch := range d.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}
