package bus

import (
	"context"
)

// Watcher represents a value that can be watched for changes.
// The Watch method returns a channel that will receive a notification whenever the value changes.
// The channel will be closed when the context is canceled.
type Watcher interface {
	Watch(ctx context.Context) <-chan struct{}
}
