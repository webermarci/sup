package bus

// Reader represents a value that can be read. The Read method returns the current value.
type Reader[V any] interface {
	Read() V
}
