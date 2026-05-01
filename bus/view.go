package bus

import (
	"context"

	"github.com/webermarci/sup"
)

// View is a simple implementation of Readable that returns the value from a provided function. It does not have any internal state and always reflects the current value from the function.
type View[V any] struct {
	*sup.BaseActor
	update func() V
}

// NewView creates a new View with the given function that provides the value to be read.
func NewView[V any](name string, update func() V) *View[V] {
	return &View[V]{
		BaseActor: sup.NewBaseActor(name),
		update:    update,
	}
}

// Read returns the current value from the View by calling the provided function.
func (v *View[V]) Read() V {
	return v.update()
}

// Run is the main loop for the View actor. Since a View is a passive observer that does not have any internal state or periodic updates, it simply waits for the context to be canceled and then exits. This allows the View to exist as long as its supervisor is alive without consuming unnecessary resources.
func (v *View[V]) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
