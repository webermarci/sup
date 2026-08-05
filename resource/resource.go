// Package resource provides supervised, serialized access to acquired
// resources.
package resource

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webermarci/sup"
)

const (
	// DefaultReleaseTimeout is the maximum duration supplied to a resource
	// release function after an execution attempt ends.
	DefaultReleaseTimeout = 5 * time.Second
)

var (
	// ErrActorRunning is returned when Run is called while the resource actor is
	// already running.
	ErrActorRunning = errors.New("resource: actor is already running")

	// ErrActorNotRunning is returned when an operation cannot be handled because
	// the resource actor has not started or has already stopped.
	ErrActorNotRunning = errors.New("resource: actor is not running")
)

// AcquireFunc creates a resource for one execution attempt. It should honor
// ctx and return a fresh resource for every attempt because the previous
// resource is released when its actor stops.
type AcquireFunc[T any] func(context.Context) (T, error)

// ReleaseFunc releases a resource after its execution attempt ends. The
// context preserves the actor context's values, ignores its cancellation, and
// has a fresh ReleaseTimeout deadline.
type ReleaseFunc[T any] func(context.Context, T) error

// CallFunc performs a serialized request/reply operation on a resource. Its
// context is canceled when either the caller or the resource actor stops.
type CallFunc[T any, R any] func(context.Context, T) (R, error)

// CastFunc performs a serialized fire-and-forget operation on a resource. Its
// context is the resource actor's execution context. An operation error
// terminates the resource actor and is handled by supervision.
type CastFunc[T any] func(context.Context, T) error

// config contains resource actor configuration.
type config struct {
	releaseTimeout time.Duration
}

// Option configures a resource actor during construction.
type Option func(*config)

// WithReleaseTimeout sets the maximum duration of the context passed to the
// release function.
func WithReleaseTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		panic("resource: release timeout must be positive")
	}

	return func(config *config) {
		config.releaseTimeout = timeout
	}
}

// Actor acquires a resource for each execution attempt and serializes Call and
// Cast operations against it until its context is canceled or a Cast
// operation fails. A supervisor can restart it to acquire a fresh resource.
type Actor[T any] struct {
	id             string
	acquire        AcquireFunc[T]
	release        ReleaseFunc[T]
	releaseTimeout time.Duration
	running        atomic.Bool

	mu       sync.Mutex
	current  *execution[T]
	started  bool
	startedC chan struct{}
}

type execution[T any] struct {
	requests chan request[T]
	ready    chan struct{}
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
var _ sup.Inspectable = (*Actor[any])(nil)

// NewActor creates a resource actor. The acquire function is called once per
// execution attempt, and the release function is called exactly once after a
// successful acquisition, including when the resource actor fails. Calls made
// before the first Run wait for the actor to become ready; calls made after an
// execution stops return ErrActorNotRunning.
func NewActor[T any](
	id string,
	acquire AcquireFunc[T],
	release ReleaseFunc[T],
	options ...Option,
) *Actor[T] {
	if id == "" {
		panic("resource: actor id cannot be empty")
	}
	if acquire == nil {
		panic("resource: acquire function cannot be nil")
	}
	if release == nil {
		panic("resource: release function cannot be nil")
	}

	config := config{
		releaseTimeout: DefaultReleaseTimeout,
	}
	for _, option := range options {
		if option == nil {
			panic("resource: option cannot be nil")
		}
		option(&config)
	}
	if config.releaseTimeout <= 0 {
		panic("resource: release timeout must be positive")
	}

	return &Actor[T]{
		id:             id,
		acquire:        acquire,
		release:        release,
		releaseTimeout: config.releaseTimeout,
		startedC:       make(chan struct{}),
	}
}

// ID returns the actor id.
func (a *Actor[T]) ID() string {
	return a.id
}

// Inspect returns the resource actor spec.
func (a *Actor[T]) Inspect() sup.Spec {
	return sup.Spec{
		Kind:         "resource",
		Dependencies: []string{},
		Metadata: map[string]any{
			"release_timeout": a.releaseTimeout,
		},
	}
}

// Run acquires one resource and processes serialized operations until the
// actor context is canceled or a Cast operation returns an error. Cancellation
// is a clean shutdown and returns nil. A Call error is returned to its caller;
// a Cast error is returned from Run for supervision.
func (a *Actor[T]) Run(ctx context.Context) (runErr error) {
	if ctx == nil {
		panic("resource: context cannot be nil")
	}
	if !a.running.CompareAndSwap(false, true) {
		return ErrActorRunning
	}

	current := &execution[T]{
		requests: make(chan request[T]),
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}
	a.begin(current)
	defer a.finish(current)

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
	close(current.ready)

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
				value, err := req.call(opCtx, value)
				stopOperation()
				req.reply <- callResult{value: value, err: err}
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
// It blocks until the actor receives the request. The caller's context controls
// both handoff and execution. Operation errors are returned to the caller and
// do not stop the resource actor.
func Call[T any, R any](
	ctx context.Context,
	actor *Actor[T],
	operation CallFunc[T, R],
) (R, error) {
	var zero R
	if ctx == nil {
		panic("resource: context cannot be nil")
	}
	if actor == nil {
		panic("resource: actor cannot be nil")
	}
	if operation == nil {
		panic("resource: operation cannot be nil")
	}

	current, err := actor.waitForExecution(ctx)
	if err != nil {
		return zero, err
	}
	if err := waitUntilReady(ctx, current); err != nil {
		return zero, err
	}

	reply := make(chan callResult, 1)
	req := request[T]{
		caller: ctx,
		reply:  reply,
		call: func(operationCtx context.Context, value T) (any, error) {
			return operation(operationCtx, value)
		},
	}
	if err := enqueue(ctx, current, req); err != nil {
		return zero, err
	}

	select {
	case result := <-reply:
		if result.value == nil {
			return zero, result.err
		}
		value, ok := result.value.(R)
		if !ok {
			panic("resource: internal call result type mismatch")
		}
		return value, result.err
	case <-current.done:
		return zero, ErrActorNotRunning
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Cast sends a fire-and-forget operation and returns once the actor receives
// it. The operation is serialized with all other operations on the actor's
// resource and may run after Cast returns. The context passed to operation is
// the actor's execution context; ctx controls handoff. If the operation
// returns an error, the resource actor stops and supervision decides whether
// to restart it.
func Cast[T any](
	ctx context.Context,
	actor *Actor[T],
	operation CastFunc[T],
) error {
	if ctx == nil {
		panic("resource: context cannot be nil")
	}
	if actor == nil {
		panic("resource: actor cannot be nil")
	}
	if operation == nil {
		panic("resource: operation cannot be nil")
	}

	current, err := actor.waitForExecution(ctx)
	if err != nil {
		return err
	}
	if err := waitUntilReady(ctx, current); err != nil {
		return err
	}

	return enqueue(ctx, current, request[T]{
		caller: ctx,
		cast:   operation,
	})
}

func (a *Actor[T]) begin(current *execution[T]) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		a.started = true
		close(a.startedC)
	}
	a.current = current
}

func (a *Actor[T]) finish(current *execution[T]) {
	close(current.done)

	a.mu.Lock()
	if a.current == current {
		a.current = nil
	}
	a.mu.Unlock()
	a.running.Store(false)
}

func (a *Actor[T]) waitForExecution(ctx context.Context) (*execution[T], error) {
	for {
		a.mu.Lock()
		current := a.current
		started := a.started
		startedC := a.startedC
		a.mu.Unlock()

		if current != nil {
			return current, nil
		}
		if started {
			return nil, ErrActorNotRunning
		}

		select {
		case <-startedC:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func waitUntilReady[T any](ctx context.Context, current *execution[T]) error {
	select {
	case <-current.ready:
		return nil
	case <-current.done:
		return ErrActorNotRunning
	case <-ctx.Done():
		return ctx.Err()
	}
}

func enqueue[T any](ctx context.Context, current *execution[T], req request[T]) error {
	select {
	case current.requests <- req:
		return nil
	case <-current.done:
		return ErrActorNotRunning
	case <-ctx.Done():
		return ctx.Err()
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
