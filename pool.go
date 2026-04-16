package sup

import "sync"

// poolKey is used as a type-keyed identity for sync.Pool lookup.
// A nil pointer of a generic type is a valid comparable value in Go,
// making (*poolKey[T])(nil) a unique key per type T in a sync.Map.
type poolKey[R any] struct{}

var globalPools sync.Map

func getPool[R any]() *sync.Pool {
	key := (*poolKey[R])(nil)

	if p, ok := globalPools.Load(key); ok {
		return p.(*sync.Pool)
	}

	p := &sync.Pool{
		New: func() any {
			return make(chan result[R], 1)
		},
	}

	actual, _ := globalPools.LoadOrStore(key, p)
	return actual.(*sync.Pool)
}

func putReplyCh[R any](pool *sync.Pool, ch chan result[R]) {
	pool.Put(ch)
}
