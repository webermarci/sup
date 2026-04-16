package sup

import (
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
)

var (
	replyPools atomic.Value // map[reflect.Type]*sync.Pool
)

func init() {
	replyPools.Store(make(map[reflect.Type]*sync.Pool))
}

func getReplyPool[R any]() *sync.Pool {
	t := reflect.TypeFor[result[R]]()

	m, _ := replyPools.Load().(map[reflect.Type]*sync.Pool)
	if p, ok := m[t]; ok {
		return p
	}

	return getReplyPoolSlow[R](t)
}

var poolMu sync.Mutex

func getReplyPoolSlow[R any](t reflect.Type) *sync.Pool {
	poolMu.Lock()
	defer poolMu.Unlock()

	m, _ := replyPools.Load().(map[reflect.Type]*sync.Pool)
	if p, ok := m[t]; ok {
		return p
	}

	p := &sync.Pool{
		New: func() any {
			return make(chan result[R], 1)
		},
	}

	newMap := make(map[reflect.Type]*sync.Pool, len(m)+1)
	maps.Copy(newMap, m)
	newMap[t] = p
	replyPools.Store(newMap)

	return p
}
