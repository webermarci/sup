package bus

// Mirror is a simple implementation of Readable that returns the value from a provided function. It does not have any internal state and always reflects the current value from the function.
type Mirror[V any] struct {
	fn func() V
}

// NewMirror creates a new Mirror with the given function that provides the value to be read.
func NewMirror[V any](fn func() V) *Mirror[V] {
	return &Mirror[V]{fn: fn}
}

// Read returns the current value from the Mirror by calling the provided function.
func (m *Mirror[V]) Read() V {
	return m.fn()
}
