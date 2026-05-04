package bus

import (
	"context"
)

// Subscriber represents a value that can be subscribed to for updates.
// The Subscribe method returns a channel that will receive the updated value whenever it changes.
// The channel will be closed when the context is canceled.
type Subscriber[V any] interface {
	Subscribe(context.Context) <-chan V
}
