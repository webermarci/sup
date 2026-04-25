package bus

// Readable represents a value that can be read. The Read method returns the current value.
type Readable[V any] interface {
	Read() V
}
