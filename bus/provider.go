package bus

import (
	"github.com/webermarci/sup"
)

// Provider represents any reactive actor that emits values.
// Trigger, Signal, Derived, and Debounce all satisfy this interface.
type Provider[V any] interface {
	sup.Actor
	Reader[V]
	Subscriber[V]
	Watcher
}
