package sup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrMailboxFull is returned when casting to a full mailbox
	ErrMailboxFull = errors.New("mailbox is full")
	// ErrMailboxClosed is returned when casting to a closed mailbox
	ErrMailboxClosed = errors.New("mailbox is closed")
)

// result is an internal wrapper to pass both a value and an error over a channel
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
// A size of 0 means unbuffered (synchronous).
func NewMailbox[T any](size int) *Mailbox[T] {
	return &Mailbox[T]{
		ch: make(chan T, size),
	}
}

// Cast sends a message asynchronously.
// It returns an error if the mailbox is full or closed.
func (m *Mailbox[T]) Cast(msg T) error {
	if m.closed.Load() {
		return ErrMailboxClosed
	}

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

// Close safely closes the mailbox. Subsequent Casts will fail.
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

// globalPools stores a sync.Pool for every unique Res type to prevent interface{} allocations.
// We use a sync.Map keyed by the generic zero value interface to ensure type safety.
var globalPools sync.Map

// getPool retrieves or initializes a sync.Pool for the specific Res type.
func getPool[Res any]() *sync.Pool {
	var zero Res
	key := any(zero)

	if p, ok := globalPools.Load(key); ok {
		return p.(*sync.Pool)
	}

	p := &sync.Pool{
		New: func() any {
			// Buffer of 1 ensures the actor never blocks when replying,
			// even if the caller times out and walks away.
			return make(chan result[Res], 1)
		},
	}
	actual, _ := globalPools.LoadOrStore(key, p)
	return actual.(*sync.Pool)
}

// Call sends a message to an actor and waits for a reply, utilizing sync.Pool for zero allocations.
func Call[Req any, Res any](ctx context.Context, mb *Mailbox[any], msg Req) (Res, error) {
	var zero Res

	// 1. Get a pre-allocated reply channel from the pool
	pool := getPool[Res]()
	replyCh := pool.Get().(chan result[Res])

	// 2. Ensure we clean up and return the channel to the pool
	defer func() {
		// Drain any abandoned messages (e.g., if ctx timed out but actor still replied)
		select {
		case <-replyCh:
		default:
		}
		pool.Put(replyCh)
	}()

	// 3. Create the request payload
	req := Request[Req, Res]{
		Msg:     msg,
		replyTo: replyCh,
	}

	// 4. Dispatch to actor
	if err := mb.Cast(req); err != nil {
		return zero, err
	}

	// 5. Await reply or timeout
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case res := <-replyCh:
		return res.val, res.err
	}
}
