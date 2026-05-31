package sup

import (
	"context"
	"strconv"
	"time"
)

// Derived is a read-only signal computed from one or more watcher signals.
type Derived[V any] struct {
	*baseSignal[V]
	compute     func() V
	deps        []WatcherSignal
	batchWindow time.Duration
}

// NewDerived creates a derived signal with the initial computed value.
func NewDerived[V any](id string, compute func() V, deps ...WatcherSignal) *Derived[V] {
	return &Derived[V]{
		baseSignal:  newBaseSignal(id, compute()),
		compute:     compute,
		deps:        deps,
		batchWindow: 5 * time.Millisecond,
	}
}

// InitialNotify makes new subscribers receive the current value immediately.
func (d *Derived[V]) InitialNotify() *Derived[V] {
	d.setInitialNotify(true)
	return d
}

// Equal sets the equality function used to suppress unchanged values.
func (d *Derived[V]) Equal(eq func(a, b V) bool) *Derived[V] {
	d.setEqual(eq)
	return d
}

// BatchWindow sets the coalescing window for dependency updates.
func (d *Derived[V]) BatchWindow(window time.Duration) *Derived[V] {
	d.mu.Lock()
	d.batchWindow = window
	d.mu.Unlock()
	return d
}

// Effect creates an effect that reacts to this derived signal.
func (d *Derived[V]) Effect(
	id string,
	fn EffectFunc[V],
) *Effect[V] {
	return NewEffect(id, d, fn)
}

// Run watches dependencies and recomputes the value until the context is canceled.
func (d *Derived[V]) Run(ctx context.Context) error {
	defer d.closeAll()
	ping := make(chan struct{}, 1)

	for _, dep := range d.deps {
		ch := dep.Watch(ctx)
		go func(c <-chan struct{}) {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-c:
					if !ok {
						return
					}
					select {
					case ping <- struct{}{}:
					default:
					}
				}
			}
		}(ch)
	}

	coalesce := time.NewTimer(time.Hour)
	if !coalesce.Stop() {
		select {
		case <-coalesce.C:
		default:
		}
	}
	var coalesceChan <-chan time.Time
	pending := false

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ping:
			if !pending {
				pending = true
				coalesce.Reset(d.batchWindow)
				coalesceChan = coalesce.C
			}

		case <-coalesceChan:
			pending = false
			coalesceChan = nil

			value := d.compute()
			d.set(value)
		}
	}
}

// Inspect returns the derived signal spec.
func (d *Derived[V]) Inspect() Spec {
	d.mu.RLock()
	batchWindow := d.batchWindow
	initialNotify := d.initialNotify

	var dependencies []string
	for _, d := range d.deps {
		dependencies = append(dependencies, d.ID())
	}
	d.mu.RUnlock()

	return Spec{
		Kind:         "derived",
		Dependencies: dependencies,
		Metadata: map[string]string{
			"batch_window":   batchWindow.String(),
			"initial_notify": strconv.FormatBool(initialNotify),
		},
	}
}
