package sup

import (
	"context"
	"sync"
)

type callContext[R any] struct {
	replyCh chan R
	errCh   chan error
}

type process[A any, S any, R any] struct {
	mailbox chan envelope[A, S, R]
	pool    *sync.Pool
}

type envelope[A any, S any, R any] struct {
	isCall  bool
	castMsg A
	callMsg S
	replyCh chan R
	errCh   chan error
}

type System struct {
	ctx      context.Context
	registry sync.Map
}

func NewSystem(ctx context.Context) *System {
	return &System{ctx: ctx}
}
