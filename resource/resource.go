package resource

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/webermarci/sup"
)

const defaultReleaseTimeout = 5 * time.Second

// ErrExecutionStopped is returned when an accepted Call cannot complete
// because its resource execution stopped before producing a reply. The
// operation is not retried because it may already have produced side effects.
var ErrExecutionStopped = errors.New("resource: execution stopped before call completed")

// AcquireFunc creates a resource for one execution attempt. It should honor
// ctx and return a fresh resource for every attempt because the previous
// resource is released when its actor stops.
type AcquireFunc[T any] func(context.Context) (T, error)

// ReleaseFunc releases a resource after its execution attempt ends. The
// context preserves the actor context's values, ignores its cancellation, and
// has a fresh ReleaseTimeout deadline.
type ReleaseFunc[T any] func(context.Context, T) error

// CallFunc performs a serialized request/reply operation on a resource. Its
// context is canceled when either the caller or the resource actor stops. An
// operation error is returned to the caller and terminates the resource actor.
type CallFunc[T any, R any] func(context.Context, T) (R, error)

// CastFunc performs a serialized fire-and-forget operation on a resource. Its
// context is the resource actor's execution context. An operation error
// terminates the resource actor and is handled by supervision.
type CastFunc[T any] func(context.Context, T) error

// Actor acquires a resource for each execution attempt and serializes Call and
// Cast operations against it until its context is canceled or an operation
// fails. A supervisor can restart it to acquire a fresh resource.
type Actor[T any] struct {
	id             string
	acquire        AcquireFunc[T]
	release        ReleaseFunc[T]
	releaseTimeout time.Duration
	mu             sync.Mutex
	current        *execution[T]
	changed        chan struct{}
}

type execution[T any] struct {
	requests chan request[T]
	done     chan struct{}
}

type request[T any] struct {
	caller context.Context
	call   func(context.Context, T) (any, error)
	cast   func(context.Context, T) error
	reply  chan callResult
}

type callResult struct {
	value any
	err   error
}

var _ sup.Actor = (*Actor[any])(nil)

// NewActor creates a resource actor. The acquire function is called once per
// execution attempt, and the release function is called exactly once after a
// successful acquisition, including when the resource actor fails. Operations
// made before the first Run and between execution attempts wait for the actor
// to become ready. A request that has not been accepted carries over to the
// next execution attempt. Release defaults to a five-second timeout.
//
// Optional configuration is added with methods before the first Run call.
func NewActor[T any](
	id string,
	acquire AcquireFunc[T],
	release ReleaseFunc[T],
) *Actor[T] {
	if id == "" {
		panic("resource: actor id cannot be empty")
	}
	return &Actor[T]{
		id:             id,
		acquire:        acquire,
		release:        release,
		releaseTimeout: defaultReleaseTimeout,
		changed:        make(chan struct{}),
	}
}

// SetReleaseTimeout sets the maximum duration of the context passed to the
// release function and returns the actor for chaining.
func (a *Actor[T]) SetReleaseTimeout(timeout time.Duration) *Actor[T] {
	if timeout <= 0 {
		panic("resource: release timeout must be positive")
	}
	a.releaseTimeout = timeout
	return a
}

// ID returns the actor id.
func (a *Actor[T]) ID() string {
	return a.id
}

// Run acquires one resource and processes serialized operations until the actor
// context is canceled or an operation returns an error. Cancellation is a clean
// shutdown and returns nil. Call errors are returned to their callers, and both
// Call and Cast errors are returned from Run for supervision.
func (a *Actor[T]) Run(ctx context.Context) (runErr error) {
	var current *execution[T]
	defer func() {
		if current != nil {
			a.finish(current)
		}
	}()

	if ctx.Err() != nil {
		return nil
	}

	value, err := a.acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	defer func() {
		releaseCtx, cancelRelease := context.WithTimeout(
			context.WithoutCancel(ctx),
			a.releaseTimeout,
		)
		defer cancelRelease()
		releaseErr := a.release(releaseCtx, value)
		if ctx.Err() != nil {
			runErr = nil
			return
		}
		if releaseErr != nil {
			runErr = errors.Join(runErr, releaseErr)
		}
	}()

	if ctx.Err() != nil {
		return nil
	}
	current = &execution[T]{
		requests: make(chan request[T]),
		done:     make(chan struct{}),
	}
	a.begin(current)

	for {
		if ctx.Err() != nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case req := <-current.requests:
			if ctx.Err() != nil {
				return nil
			}

			if req.call != nil {
				if req.caller.Err() != nil {
					req.reply <- callResult{err: req.caller.Err()}
					continue
				}

				opCtx, stopOperation := operationContext(ctx, req.caller)
				result, err := req.call(opCtx, value)
				stopOperation()
				req.reply <- callResult{value: result, err: err}
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
				continue
			}

			if err := req.cast(ctx, value); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// Call sends a request, waits for its typed result, and returns it. The
// operation is serialized with all other operations on the actor's resource.
// It waits across execution attempts until the actor receives the request. The
// caller's context controls both handoff and execution. Operation errors are
// returned to the caller and stop the resource actor so supervision can decide
// whether to restart it.
func Call[T any, R any](
	ctx context.Context,
	actor *Actor[T],
	operation CallFunc[T, R],
) (R, error) {
	var zero R
	reply := make(chan callResult, 1)
	req := request[T]{
		caller: ctx,
		reply:  reply,
		call: func(operationCtx context.Context, value T) (any, error) {
			return operation(operationCtx, value)
		},
	}
	current, err := actor.enqueue(ctx, req)
	if err != nil {
		return zero, err
	}

	select {
	case result := <-reply:
		return unpackCallResult[R](result)
	case <-current.done:
		select {
		case result := <-reply:
			return unpackCallResult[R](result)
		default:
		}
		return zero, ErrExecutionStopped
	case <-ctx.Done():
		select {
		case result := <-reply:
			return unpackCallResult[R](result)
		default:
		}
		return zero, ctx.Err()
	}
}

func unpackCallResult[R any](result callResult) (R, error) {
	var zero R
	if result.value == nil {
		return zero, result.err
	}
	value, ok := result.value.(R)
	if !ok {
		panic("resource: internal call result type mismatch")
	}
	return value, result.err
}

// Cast sends a fire-and-forget operation, waits across execution attempts, and
// returns once the actor receives it. The operation is serialized with all
// other operations on the actor's resource and may run after Cast returns. The
// context passed to operation is the actor's execution context; ctx controls
// handoff. If the operation returns an error, the resource actor stops and
// supervision decides whether to restart it.
func Cast[T any](
	ctx context.Context,
	actor *Actor[T],
	operation CastFunc[T],
) error {
	_, err := actor.enqueue(ctx, request[T]{
		cast: operation,
	})
	return err
}

func (a *Actor[T]) begin(current *execution[T]) {
	a.mu.Lock()
	a.current = current
	a.notifyChangedLocked()
	a.mu.Unlock()
}

func (a *Actor[T]) finish(current *execution[T]) {
	a.mu.Lock()
	close(current.done)
	if a.current == current {
		a.current = nil
		a.notifyChangedLocked()
	}
	a.mu.Unlock()
}

func (a *Actor[T]) notifyChangedLocked() {
	close(a.changed)
	a.changed = make(chan struct{})
}

func (a *Actor[T]) enqueue(
	ctx context.Context,
	req request[T],
) (*execution[T], error) {
	for {
		a.mu.Lock()
		current := a.current
		changed := a.changed
		a.mu.Unlock()

		if current == nil {
			select {
			case <-changed:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		select {
		case current.requests <- req:
			return current, nil
		case <-current.done:
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func operationContext(actorCtx, callerCtx context.Context) (context.Context, func()) {
	operationCtx, cancel := context.WithCancel(callerCtx)
	stop := context.AfterFunc(actorCtx, cancel)
	return operationCtx, func() {
		stop()
		cancel()
	}
}
