package bus

import (
	"context"
	"errors"

	"github.com/webermarci/sup"
)

type streamGetValueMessage struct{}

// Stream consumes values from a channel provided by a factory function and broadcasts them to subscribers.
// If the factory returns an error or the channel closes, the actor exits with an error, allowing a supervisor to handle the restart.
type Stream[V any] struct {
	*sup.BaseActor
	broadcaster   broadcaster[V]
	mailbox       *sup.Mailbox
	value         V
	factory       func(context.Context) (<-chan V, error)
	initialNotify bool
}

// NewStream creates a new Stream with the given name and source factory function.
func NewStream[V any](name string, factory func(context.Context) (<-chan V, error)) *Stream[V] {
	return &Stream[V]{
		BaseActor: sup.NewBaseActor(name),
		broadcaster: broadcaster[V]{
			buffer: 16,
		},
		mailbox: sup.NewMailbox(64),
		factory: factory,
	}
}

// WithMailboxSize allows configuring the mailbox buffer size for the Stream.
func (s *Stream[V]) WithMailboxSize(size int) *Stream[V] {
	s.mailbox = sup.NewMailbox(size)
	return s
}

// WithInitialValue sets the initial value of the Stream.
func (s *Stream[V]) WithInitialValue(initial V) *Stream[V] {
	s.value = initial
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (s *Stream[V]) WithSubscriberBuffer(buffer int) *Stream[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (s *Stream[V]) WithInitialNotify(enabled bool) *Stream[V] {
	s.initialNotify = enabled
	return s
}

// Read returns the last value received from the stream.
func (s *Stream[V]) Read() V {
	res, _ := sup.Call[streamGetValueMessage, V](s.mailbox, streamGetValueMessage{})
	return res
}

// Subscribe allows clients to receive updates whenever a new value arrives on the stream.
func (s *Stream[V]) Subscribe(ctx context.Context) <-chan V {
	return s.broadcaster.subscribe(ctx, s.mailbox)
}

// Run starts the Stream's main loop. It attempts to acquire the source channel and then processes incoming messages and stream values.
// If the factory fails or the source channel closes, it returns an error.
func (s *Stream[V]) Run(ctx context.Context) error {
	source, err := s.factory(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			s.broadcaster.closeAll()
			return nil

		case msg, ok := <-s.mailbox.Receive():
			if !ok {
				s.broadcaster.closeAll()
				return nil
			}

			switch m := msg.(type) {
			case sup.CallRequest[streamGetValueMessage, V]:
				m.Reply(s.value, nil)

			default:
				s.broadcaster.handleSubscription(msg, s.value, s.initialNotify)
			}

		case v, ok := <-source:
			if !ok {
				s.broadcaster.closeAll()
				return errors.New("stream source closed")
			}
			s.value = v
			s.broadcaster.notify(v)
		}
	}
}
