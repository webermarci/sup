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
	value Res
	err   error
}

// Mailbox is a thread-safe message queue for actors.
type Mailbox struct {
	ch     chan any
	closed atomic.Bool
}

// NewMailbox creates a new mailbox with the specified buffer size.
// A size of 0 means unbuffered.
func NewMailbox(size int) *Mailbox {
	return &Mailbox{
		ch: make(chan any, size),
	}
}

// Len returns the current number of messages in the mailbox buffer.
func (m *Mailbox) Len() int {
	return len(m.ch)
}

// Cap returns the total capacity of the mailbox buffer.
func (m *Mailbox) Cap() int {
	return cap(m.ch)
}

// IsClosed checks if the mailbox has been closed.
func (m *Mailbox) IsClosed() bool {
	return m.closed.Load()
}

// Cast sends a message, waiting until it can be enqueued or the mailbox is closed.
func (m *Mailbox) Cast(message any) error {
	return m.CastContext(context.Background(), message)
}

// CastContext sends a message with context support for cancellation and timeouts.
func (m *Mailbox) CastContext(ctx context.Context, message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryCast attempts to send a message without blocking.
// It returns ErrMailboxFull immediately if the mailbox buffer is full.
func (m *Mailbox) TryCast(message any) (err error) {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMailboxClosed
		}
	}()

	select {
	case m.ch <- message:
		return nil
	default:
		return ErrMailboxFull
	}
}

// Receive returns the read-only channel to consume messages.
func (m *Mailbox) Receive() <-chan any {
	return m.ch
}

// Close safely closes the mailbox. Subsequent sends fail.
func (m *Mailbox) Close() {
	if m.closed.CompareAndSwap(false, true) {
		close(m.ch)
	}
}

// Request wraps a message with a reply channel for synchronous calls.
// replyTo is always set when constructed via Call or TryCall.
type Request[Req any, Res any] struct {
	Message Req
	replyTo chan result[Res]
}

// Reply sends the response back to the caller.
func (r *Request[Req, Res]) Reply(val Res, err error) {
	select {
	case r.replyTo <- result[Res]{value: val, err: err}:
	default:
		panic("sup: Reply called more than once on the same Request")
	}
}

// poolKey is used as a type-keyed identity for sync.Pool lookup.
// A nil pointer of a generic type is a valid comparable value in Go,
// making (*poolKey[T])(nil) a unique key per type T in a sync.Map.
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
	pool.Put(ch)
}

// Call sends a message to an actor and waits indefinitely for a reply.
func Call[Req any, Res any](mb *Mailbox, message Req) (Res, error) {
	return CallContext[Req, Res](context.Background(), mb, message)
}

// CallContext sends a message to an actor and waits for a reply until the context expires.
func CallContext[Req any, Res any](ctx context.Context, mb *Mailbox, message Req) (Res, error) {
	var zero Res

	pool := getPool[Res]()
	replyCh := pool.Get().(chan result[Res])

	req := Request[Req, Res]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.CastContext(ctx, req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	select {
	case res := <-replyCh:
		putReplyCh(pool, replyCh)
		return res.value, res.err
	case <-ctx.Done():
		// The actor may still call Reply() after the context expires.
		// Drain the channel in a goroutine so the actor never blocks.
		go func() {
			<-replyCh
			putReplyCh(pool, replyCh)
		}()
		return zero, ctx.Err()
	}
}

// TryCall attempts to enqueue a request without blocking.
// If the enqueue succeeds, it waits indefinitely for the reply.
// Use TryCallContext if an escape hatch is needed while waiting for the reply.
func TryCall[Req any, Res any](mb *Mailbox, message Req) (Res, error) {
	return TryCallContext[Req, Res](context.Background(), mb, message)
}

// TryCallContext attempts to enqueue a request without blocking.
// If the enqueue succeeds, it waits for a reply until the context expires.
func TryCallContext[Req any, Res any](ctx context.Context, mb *Mailbox, message Req) (Res, error) {
	var zero Res

	pool := getPool[Res]()
	replyCh := pool.Get().(chan result[Res])

	req := Request[Req, Res]{
		Message: message,
		replyTo: replyCh,
	}

	if err := mb.TryCast(req); err != nil {
		putReplyCh(pool, replyCh)
		return zero, err
	}

	select {
	case res := <-replyCh:
		putReplyCh(pool, replyCh)
		return res.value, res.err
	case <-ctx.Done():
		go func() {
			<-replyCh
			putReplyCh(pool, replyCh)
		}()
		return zero, ctx.Err()
	}
}
