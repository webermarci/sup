package sup

import (
	"maps"
	"reflect"
	"sync"
	"sync/atomic"
)

var (
	replyPools   atomic.Value // map[reflect.Type]*sync.Pool
	requestPools sync.Map     // map[reflect.Type]*requestPool
)

func init() {
	replyPools.Store(make(map[reflect.Type]*sync.Pool))
}

type requestPool struct {
	pool sync.Pool
}

func (p *requestPool) Get() any {
	return p.pool.Get()
}

func (p *requestPool) Put(x any) {
	p.pool.Put(x)
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

func getCallRequestPool[T any, R any]() *requestPool {
	t := reflect.TypeFor[CallRequest[T, R]]()
	if p, ok := requestPools.Load(t); ok {
		return p.(*requestPool)
	}

	p, _ := requestPools.LoadOrStore(t, &requestPool{
		pool: sync.Pool{
			New: func() any {
				return new(CallRequest[T, R])
			},
		},
	})
	return p.(*requestPool)
}
