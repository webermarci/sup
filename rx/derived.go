package rx

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/webermarci/sup"
)

// ErrDerivedRunning is returned when Run is called while the derived value is
// already running.
var ErrDerivedRunning = errors.New("rx: derived is already running")

// Derived is read-only reactive state computed from one or more dependencies.
//
// Unlike Signal, Derived is an actor: Run watches its dependencies and
// maintains its shared computed value. Derived dependency graphs must be
// acyclic.
type Derived[V any] struct {
	value   *Signal[V]
	compute func() V
	deps    []Dependency

	running bool
	runMu   sync.Mutex
}

// NewDerived creates a derived value with an initial computed value.
func NewDerived[V any](
	id string,
	compute func() V,
	dependencies ...Dependency,
) *Derived[V] {
	if id == "" {
		panic("rx: derived id cannot be empty")
	}
	if compute == nil {
		panic("rx: derived compute function cannot be nil")
	}

	deps := slices.Clone(dependencies)
	for _, dependency := range deps {
		if dependency == nil {
			panic("rx: derived dependency cannot be nil")
		}
	}

	return &Derived[V]{
		value:   NewSignal(id, compute()),
		compute: compute,
		deps:    deps,
	}
}

// ID returns the derived value id.
func (d *Derived[V]) ID() string {
	return d.value.ID()
}

// Value returns the current derived value.
func (d *Derived[V]) Value() V {
	return d.value.Value()
}

// Subscribe returns an independent, coalescing stream of derived values.
func (d *Derived[V]) Subscribe(ctx context.Context) <-chan V {
	return d.value.Subscribe(ctx)
}

// Watch returns an independent, coalescing stream of change notifications.
func (d *Derived[V]) Watch(ctx context.Context) <-chan struct{} {
	return d.value.Watch(ctx)
}

// Run watches dependencies and recomputes the value until ctx is canceled.
func (d *Derived[V]) Run(ctx context.Context) error {
	if ctx == nil {
		panic("rx: derived context cannot be nil")
	}
	if !d.beginRun() {
		return ErrDerivedRunning
	}
	defer d.endRun()

	runCtx, cancel := context.WithCancel(ctx)
	changed := make(chan struct{}, 1)
	var watchers sync.WaitGroup
	defer func() {
		cancel()
		watchers.Wait()
	}()

	for _, dependency := range d.deps {
		notifications := dependency.Watch(runCtx)

		// Every dependency watch starts with one notification. Consume it here
		// so all watches are established before the single startup refresh.
		select {
		case _, ok := <-notifications:
			if !ok {
				return nil
			}
		case <-runCtx.Done():
			return nil
		}

		watchers.Go(func() {
			for range notifications {
				select {
				case changed <- struct{}{}:
				default:
				}
			}
		})
	}

	if len(d.deps) > 0 {
		if runCtx.Err() != nil {
			return nil
		}
		d.value.Set(d.compute())
	}

	for {
		select {
		case <-runCtx.Done():
			return nil
		case <-changed:
			if runCtx.Err() != nil {
				return nil
			}
			d.value.Set(d.compute())
		}
	}
}

// Inspect returns the derived value spec.
func (d *Derived[V]) Inspect() sup.Spec {
	dependencies := make([]string, 0, len(d.deps))
	for _, dependency := range d.deps {
		dependencies = append(dependencies, dependency.ID())
	}

	return sup.Spec{
		Kind:         "derived",
		Dependencies: dependencies,
		Metadata:     map[string]any{},
	}
}

func (d *Derived[V]) beginRun() bool {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	if d.running {
		return false
	}
	d.running = true
	return true
}

func (d *Derived[V]) endRun() {
	d.runMu.Lock()
	d.running = false
	d.runMu.Unlock()
}
