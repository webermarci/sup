package sup

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
)

// ErrUnexpectedResponseType is returned when a call receives a successful
// response of a different type than the caller requested.
var ErrUnexpectedResponseType = errors.New("sup: unexpected call response type")

type result[R any] struct {
	value R
	err   error
}

// CallRequest wraps a payload with a reply operation for synchronous calls.
type CallRequest[T any, R any] struct {
	ctx     context.Context
	payload T
	reply   func(R, error) bool
}

// Context returns the caller's context.
func (r CallRequest[T, R]) Context() context.Context {
	return r.ctx
}

// Payload returns the request's payload.
func (r CallRequest[T, R]) Payload() T {
	return r.payload
}

// Reply sends the response back to the caller. It returns false when the caller
// has canceled or a reply was already sent. A successful response of a type the
// caller did not request is accepted and delivered as ErrUnexpectedResponseType.
func (r CallRequest[T, R]) Reply(value R, err error) bool {
	return r.reply(value, err)
}

func newCallReply[R, S any](ctx context.Context, replyTo chan<- result[S]) func(R, error) bool {
	replied := &atomic.Bool{}
	return func(value R, err error) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if !replied.CompareAndSwap(false, true) {
			return false
		}

		response := result[S]{err: err}
		if err == nil {
			untyped := any(value)
			typed, ok := untyped.(S)
			if !ok && untyped == nil && nilable[S]() {
				ok = true
			}
			if !ok {
				response.err = fmt.Errorf(
					"%w: got %T, want %v",
					ErrUnexpectedResponseType,
					value,
					reflect.TypeFor[S](),
				)
			} else {
				response.value = typed
			}
		}

		select {
		case replyTo <- response:
			return true
		case <-ctx.Done():
			return false
		}
	}
}

func nilable[T any]() bool {
	switch reflect.TypeFor[T]().Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}
