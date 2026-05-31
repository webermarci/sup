package sup

import "context"

// EffectFunc handles a value emitted by a signal.
type EffectFunc[V any] func(context.Context, V) error

// Effect is an actor that reacts to values from a ReadableSignal.
type Effect[V any] struct {
	*BaseActor
	signal ReadableSignal[V]
	fn     EffectFunc[V]
}

// NewEffect creates an effect that reacts to values from signal.
func NewEffect[V any](
	id string,
	signal ReadableSignal[V],
	fn EffectFunc[V],
) *Effect[V] {
	if signal == nil {
		panic("sup: effect signal cannot be nil")
	}

	if fn == nil {
		panic("sup: effect function cannot be nil")
	}

	return &Effect[V]{
		BaseActor: NewBaseActor(id),
		signal:    signal,
		fn:        fn,
	}
}

// Run subscribes to the signal and calls the effect function for each value.
func (e *Effect[V]) Run(ctx context.Context) error {
	values := e.signal.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil

		case value, ok := <-values:
			if !ok {
				return nil
			}

			if err := e.fn(ctx, value); err != nil {
				return err
			}
		}
	}
}

// Inspect returns the effect spec.
func (e *Effect[V]) Inspect() Spec {
	return Spec{
		Kind:         "effect",
		Dependencies: []string{e.signal.ID()},
		Metadata:     map[string]string{},
	}
}
