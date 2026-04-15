package sup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrMailboxFull is returned by TryCast when the mailbox buffer is full.
	ErrMailboxFull = errors.New("mailbox is full")
	// ErrMailboxClosed is returned when trying to send to a closed mailbox.
	ErrMailboxClosed = errors.New("mailbox is closed")
)

type result[Res any] struct {
	val Res
	err error
}

// Mailbox is a generic, thread-safe message queue for actors.
type Mailbox[T any] struct {
	ch     chan T
	closed atomic.Bool
}

// NewMailbox creates a new mailbox with the specified buffer size.
// A size of 0 means unbuffered.
func NewMailbox[T any](size int) *Mailbox[T] {
	return &Mailbox[T]{
		ch: make(chan T, size),
	}
}

// Len returns the current number of messages in the mailbox buffer.
func (m *Mailbox[T]) Len() int {
	return len(m.ch)
}

// Cap returns the total capacity of the mailbox buffer.
func (m *Mailbox[T]) Cap() int {
	return cap(m.ch)
}

// IsClosed checks if the mailbox has been closed.
func (m *Mailbox[T]) IsClosed() bool {
	return m.closed.Load()
}

// Cast sends a message, waiting until it can be enqueued or the mailbox is closed.
func (m *Mailbox[T]) Cast(msg T) error {
	return m.CastContext(context.Background(), msg)
}

// CastContext sends a message with context support for cancellation and timeouts.
func (m *Mailbox[T]) CastContext(ctx context.Context, msg T) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryCast attempts to send a message without blocking.
// It returns ErrMailboxFull immediately if the mailbox buffer is full.
func (m *Mailbox[T]) TryCast(msg T) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- msg:
		return nil
	default:
		return ErrMailboxFull
	}
}

// Receive returns the read-only channel to consume messages.
func (m *Mailbox[T]) Receive() <-chan T {
	return m.ch
}

// Close safely closes the mailbox. Subsequent sends fail.
func (m *Mailbox[T]) Close() {
	if m.closed.CompareAndSwap(false, true) {
		close(m.ch)
	}
}

// Request wraps a message with a reply channel for synchronous calls.
type Request[Req any, Res any] struct {
	Msg     Req
	replyTo chan result[Res]
}

// Reply sends the response back to the caller.
func (r *Request[Req, Res]) Reply(val Res, err error) {
	if r.replyTo != nil {
		r.replyTo <- result[Res]{val: val, err: err}
	}
}

type poolKey[Res any] struct{}

var globalPools sync.Map

func getPool[Res any]() *sync.Pool {
	key := (*poolKey[Res])(nil)

	if p, ok := globalPools.Load(key); ok {
		return p.(*sync.Pool)
	}

	p := &sync.Pool{
		New: func() any {
			return make(chan result[Res], 1)
		},
	}

	actual, _ := globalPools.LoadOrStore(key, p)
	return actual.(*sync.Pool)
}

func putReplyCh[Res any](pool *sync.Pool, ch chan result[Res]) {
	select {
	case <-ch:
	default:
	}
	pool.Put(ch)
}

// Call sends a message to an actor and waits indefinitely for a reply.
func Call[Req any, Res any](mb *Mailbox[any], msg Req) (Res, error) {
	return CallContext[Req, Res](context.Background(), mb, msg)
}

// CallContext sends a message to an actor and waits for a reply until the context expires.
func CallContext[Req any, Res any](ctx context.Context, mb *Mailbox[any], msg Req) (Res, error) {
	var zero Res

	pool := getPool[Res]()
	replyCh := pool.Get().(chan result[Res])

	req := Request[Req, Res]{
		Msg:     msg,
		replyTo: replyCh,
	}

	if err := mb.CastContext(ctx, any(req)); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	select {
	case res := <-replyCh:
		putReplyCh(pool, replyCh)
		return res.val, res.err
	case <-ctx.Done():
		// Do not return replyCh to the pool here.
		// The actor may still reply later.
		return zero, ctx.Err()
	}
}

// TryCall attempts to enqueue a request immediately.
// If enqueue succeeds, it waits indefinitely for the reply.
func TryCall[Req any, Res any](mb *Mailbox[any], msg Req) (Res, error) {
	var zero Res

	pool := getPool[Res]()
	replyCh := pool.Get().(chan result[Res])

	req := Request[Req, Res]{
		Msg:     msg,
		replyTo: replyCh,
	}

	if err := mb.TryCast(any(req)); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	res := <-replyCh
	putReplyCh(pool, replyCh)
	return res.val, res.err
}
