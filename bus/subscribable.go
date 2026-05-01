package bus

import "context"

type Subscribable[V any] interface {
	Subscribe(context.Context) <-chan V
}
