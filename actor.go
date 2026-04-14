package sup

import (
	"context"
	"errors"
	"sync/atomic"
)

var (
	ErrNotImplemented  = errors.New("method not implemented")
	ErrProcessNotFound = errors.New("process not found")
)

type Actor[I comparable, A any, S any, R any] interface {
	PID() I
	OnInit(ctx context.Context) error
	ReceiveCast(ctx context.Context, msg A)
	ReceiveCall(ctx context.Context, msg S) (R, error)
	OnTerminate(reason error)
	Stop()
	IsStopped() bool
}

type Producer[I comparable, A any, S any, R any] func() Actor[I, A, S, R]

type BaseActor[I comparable, A any, S any, R any] struct {
	ID      I
	stopped atomic.Bool
}

func (b *BaseActor[I, A, S, R]) PID() I {
	return b.ID
}

func (b *BaseActor[I, A, S, R]) OnInit(ctx context.Context) error {
	return nil
}

func (b *BaseActor[I, A, S, R]) ReceiveCast(ctx context.Context, msg A) {}

func (b *BaseActor[I, A, S, R]) ReceiveCall(ctx context.Context, msg S) (R, error) {
	var zero R
	return zero, ErrNotImplemented
}

func (b *BaseActor[I, A, S, R]) OnTerminate(reason error) {}

func (b *BaseActor[I, A, S, R]) Stop() {
	b.stopped.Store(true)
}

func (b *BaseActor[I, A, S, R]) IsStopped() bool {
	return b.stopped.Load()
}
