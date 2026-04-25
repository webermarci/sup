package bus

import (
	"context"
	"time"

	"github.com/webermarci/sup"
)

type signalGetValueMessage struct{}

// Signal represents a value that is periodically updated by a function and can be subscribed to for updates.
type Signal[V any] struct {
	broadcaster   broadcaster[V]
	mailbox       *sup.Mailbox
	value         V
	update        func() (V, error)
	interval      time.Duration
	initialNotify bool
}

// NewSignal creates a new Signal with the given update function.
func NewSignal[V any](update func() (V, error)) *Signal[V] {
	return &Signal[V]{
		broadcaster: broadcaster[V]{
			buffer: 16,
		},
		mailbox:  sup.NewMailbox(64),
		update:   update,
		interval: time.Second,
	}
}

// WithMailboxSize allows configuring the mailbox buffer size for the Signal.
func (s *Signal[V]) WithMailboxSize(size int) *Signal[V] {
	s.mailbox = sup.NewMailbox(size)
	return s
}

// WithInitialValue sets the initial value of the Signal before any updates occur.
func (s *Signal[V]) WithInitialValue(initial V) *Signal[V] {
	s.value = initial
	return s
}

// WithInterval sets the interval at which the Signal's update function is called to refresh its value.
func (s *Signal[V]) WithInterval(interval time.Duration) *Signal[V] {
	s.interval = interval
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (s *Signal[V]) WithSubscriberBuffer(buffer int) *Signal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (s *Signal[V]) WithInitialNotify(enabled bool) *Signal[V] {
	s.initialNotify = enabled
	return s
}

// Read returns the current value of the Signal by sending a call message to its mailbox.
func (s *Signal[V]) Read() V {
	res, _ := sup.Call[signalGetValueMessage, V](s.mailbox, signalGetValueMessage{})
	return res
}

// Subscribe allows clients to receive updates whenever the Signal's value changes. It returns a channel that will receive new values.
func (s *Signal[V]) Subscribe(ctx context.Context) <-chan V {
	return s.broadcaster.subscribe(ctx, s.mailbox)
}

// Run starts the main loop of the Signal, which periodically updates its value by calling the provided function and notifies subscribers of changes. It also handles incoming messages for getting the current value and managing subscriptions.
func (s *Signal[V]) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

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
			case sup.CallRequest[signalGetValueMessage, V]:
				m.Reply(s.value, nil)

			case sup.CallRequest[subscribeMessage[V], error]:
				ch := m.Payload().ch
				s.broadcaster.add(ch)
				if s.initialNotify {
					select {
					case ch <- s.value:
					default:
					}
				}
				m.Reply(nil, nil)

			case sup.CastRequest[unsubscribeMessage[V]]:
				s.broadcaster.remove(m.Payload().ch)
			}

		case <-ticker.C:
			value, err := s.update()
			if err != nil {
				continue
			}
			s.value = value
			s.broadcaster.notify(value)
		}
	}
}
