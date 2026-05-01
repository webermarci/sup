package bus

import "context"

type Subscriber[V any] interface {
	Subscribe(context.Context) <-chan V
}
