package bus

// ViewFunc is an adapter to allow the use of ordinary functions as Readers.
// If f is a function with the appropriate signature, ViewFunc(f) is a Reader that calls f.
type ViewFunc[V any] func() V

// Read calls the underlying function to satisfy the Reader interface.
func (f ViewFunc[V]) Read() V {
	return f()
}
