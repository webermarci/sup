package bus

import (
	"context"

	"github.com/webermarci/sup"
)

type targetGetValueMessage struct{}

type targetSetValueMessage[V any] struct {
	value V
}

type targetSyncMessage struct{}

// Target represents a value that can be updated by a function and subscribed to for updates.
type Target[V any] struct {
	broadcaster   broadcaster[V]
	mailbox       *sup.Mailbox
	value         V
	update        func(V) error
	initialNotify bool
}

// NewTarget creates a new Target with the given update function.
func NewTarget[V any](update func(V) error) *Target[V] {
	return &Target[V]{
		broadcaster: broadcaster[V]{
			buffer: 16,
		},
		mailbox: sup.NewMailbox(64),
		update:  update,
	}
}

// WithMailboxSize allows configuring the mailbox buffer size for the Target.
func (t *Target[V]) WithMailboxSize(size int) *Target[V] {
	t.mailbox = sup.NewMailbox(size)
	return t
}

// WithInitialValue sets the initial value of the Target before any updates occur.
func (t *Target[V]) WithInitialValue(initial V) *Target[V] {
	t.value = initial
	return t
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (t *Target[V]) WithSubscriberBuffer(buffer int) *Target[V] {
	t.broadcaster.buffer = buffer
	return t
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (t *Target[V]) WithInitialNotify(enabled bool) *Target[V] {
	t.initialNotify = enabled
	return t
}

// Value retrieves the current value of the Target.
func (t *Target[V]) Value() V {
	res, _ := sup.Call[targetGetValueMessage, V](t.mailbox, targetGetValueMessage{})
	return res
}

// SetValue attempts to update the Target's value using the provided update function.
func (t *Target[V]) SetValue(value V) error {
	_, err := sup.Call[targetSetValueMessage[V], error](t.mailbox, targetSetValueMessage[V]{value: value})
	return err
}

// Subscribe returns a channel that receives updates whenever the Target's value changes. The subscription is automatically cleaned up when the context is done.
func (t *Target[V]) Subscribe(ctx context.Context) <-chan V {
	return t.broadcaster.subscribe(ctx, t.mailbox)
}

// Sync forces the Target to re-evaluate its current value by calling the update function with the current value. This can be used to trigger updates to subscribers even if the value hasn't changed.
func (t *Target[V]) Sync() error {
	_, err := sup.Call[targetSyncMessage, error](t.mailbox, targetSyncMessage{})
	return err
}

// Run starts the Target's main loop, processing incoming messages. It should be run in a separate goroutine and will continue until the context is canceled or the mailbox is closed.
func (t *Target[V]) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			t.broadcaster.closeAll()
			return nil

		case msg, ok := <-t.mailbox.Receive():
			if !ok {
				t.broadcaster.closeAll()
				return nil
			}

			switch m := msg.(type) {
			case sup.CallRequest[targetGetValueMessage, V]:
				m.Reply(t.value, nil)

			case sup.CallRequest[targetSetValueMessage[V], error]:
				value := m.Payload().value
				err := t.update(value)
				if err == nil {
					t.value = value
					t.broadcaster.notify(value)
				}
				m.Reply(nil, err)

			case sup.CallRequest[subscribeMessage[V], error]:
				ch := m.Payload().ch
				t.broadcaster.add(ch)
				if t.initialNotify {
					select {
					case ch <- t.value:
					default:
					}
				}
				m.Reply(nil, nil)

			case sup.CastRequest[unsubscribeMessage[V]]:
				ch := m.Payload().ch
				t.broadcaster.remove(ch)

			case sup.CallRequest[targetSyncMessage, error]:
				err := t.update(t.value)
				if err == nil {
					t.broadcaster.notify(t.value)
				}
				m.Reply(nil, err)
			}
		}
	}
}
