package bus

import "context"

type Notifyable interface {
	Notify(ctx context.Context) <-chan struct{}
}
