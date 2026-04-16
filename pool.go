package sup

import (
	"reflect"
	"sync"
)

var replyPools sync.Map

func getReplyPool[R any]() *sync.Pool {
	t := reflect.TypeFor[result[R]]()
	if p, ok := replyPools.Load(t); ok {
		return p.(*sync.Pool)
	}

	p, _ := replyPools.LoadOrStore(t, &sync.Pool{
		New: func() any {
			return make(chan result[R], 1)
		},
	})
	return p.(*sync.Pool)
}
