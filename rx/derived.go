package rx

import (
	"context"
	"sync"
)

// Derived is read-only reactive state computed from one or more dependencies.
//
// Unlike Signal, Derived is an actor: Run watches its dependencies and
// maintains its shared computed value. Derived dependency graphs must be
// acyclic.
type Derived[V any] struct {
	value   *Signal[V]
	compute func() V
	deps    []Dependency
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

	return &Derived[V]{
		value:   NewSignal(id, compute()),
		compute: compute,
		deps:    dependencies,
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
