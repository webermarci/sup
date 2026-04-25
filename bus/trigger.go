package bus

import (
	"context"

	"github.com/webermarci/sup"
)

type triggerGetValueMessage struct{}

type triggerSetValueMessage[V any] struct {
	value V
}

type triggerSyncMessage struct{}

// Trigger represents a value that can be updated by a function and subscribed to for updates.
type Trigger[V any] struct {
	broadcaster   broadcaster[V]
	mailbox       *sup.Mailbox
	value         V
	update        func(V) error
	initialNotify bool
}

// NewTrigger creates a new Trigger with the given update function.
func NewTrigger[V any](update func(V) error) *Trigger[V] {
	return &Trigger[V]{
		broadcaster: broadcaster[V]{
			buffer: 16,
		},
		mailbox: sup.NewMailbox(64),
		update:  update,
	}
}

// WithMailboxSize allows configuring the mailbox buffer size for the Trigger.
func (t *Trigger[V]) WithMailboxSize(size int) *Trigger[V] {
	t.mailbox = sup.NewMailbox(size)
	return t
}

// WithInitialValue sets the initial value of the Trigger before any updates occur.
func (t *Trigger[V]) WithInitialValue(initial V) *Trigger[V] {
	t.value = initial
	return t
}

// WithSubscriberBuffer configures the buffer size for subscriber channels to prevent blocking on updates.
func (t *Trigger[V]) WithSubscriberBuffer(buffer int) *Trigger[V] {
	t.broadcaster.buffer = buffer
	return t
}

// WithInitialNotify configures whether new subscribers should receive the current value immediately upon subscribing.
func (t *Trigger[V]) WithInitialNotify(enabled bool) *Trigger[V] {
	t.initialNotify = enabled
	return t
}

// Value retrieves the current value of the Trigger.
func (t *Trigger[V]) Value() V {
	res, _ := sup.Call[triggerGetValueMessage, V](t.mailbox, triggerGetValueMessage{})
	return res
}

// SetValue attempts to update the Trigger's value using the provided update function.
func (t *Trigger[V]) SetValue(value V) error {
	_, err := sup.Call[triggerSetValueMessage[V], error](t.mailbox, triggerSetValueMessage[V]{value: value})
	return err
}

// Subscribe returns a channel that receives updates whenever the Trigger's value changes. The subscription is automatically cleaned up when the context is done.
func (t *Trigger[V]) Subscribe(ctx context.Context) <-chan V {
	return t.broadcaster.subscribe(ctx, t.mailbox)
}

// Sync forces the Trigger to re-evaluate its current value by calling the update function with the current value. This can be used to trigger updates to subscribers even if the value hasn't changed.
func (t *Trigger[V]) Sync() error {
	_, err := sup.Call[triggerSyncMessage, error](t.mailbox, triggerSyncMessage{})
	return err
}

// Run starts the Trigger's main loop, processing incoming messages. It should be run in a separate goroutine and will continue until the context is canceled or the mailbox is closed.
func (t *Trigger[V]) Run(ctx context.Context) error {
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
			case sup.CallRequest[triggerGetValueMessage, V]:
				m.Reply(t.value, nil)

			case sup.CallRequest[triggerSetValueMessage[V], error]:
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

			case sup.CallRequest[triggerSyncMessage, error]:
				err := t.update(t.value)
				if err == nil {
					t.broadcaster.notify(t.value)
				}
				m.Reply(nil, err)
			}
		}
	}
}
