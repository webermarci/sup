package sup

import (
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
)

var replyPools atomic.Value // stores map[reflect.Type]*sync.Pool

func init() {
	replyPools.Store(make(map[reflect.Type]*sync.Pool))
}

func getReplyPool[R any]() *sync.Pool {
	t := reflect.TypeFor[result[R]]()

	// Optimized read path: simple map lookup from an atomic value.
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

	// Double-check under lock.
	m, _ := replyPools.Load().(map[reflect.Type]*sync.Pool)
	if p, ok := m[t]; ok {
		return p
	}

	p := &sync.Pool{
		New: func() any {
			// Buffered channel (size 1) prevents producers from blocking
			// if the consumer has already timed out or returned.
			return make(chan result[R], 1)
		},
	}

	// Copy-on-write update to ensure thread safety for readers.
	newMap := make(map[reflect.Type]*sync.Pool, len(m)+1)
	maps.Copy(newMap, m)
	newMap[t] = p
	replyPools.Store(newMap)

	return p
}
