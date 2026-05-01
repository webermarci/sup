package bus

// Writer represents a value that can be updated by writing to it. The Write method may return an error if the update is rejected.
type Writer[V any] interface {
	Write(V) error
}
