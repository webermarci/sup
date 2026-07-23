package mesh

import (
	"context"
	"errors"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/webermarci/sup"
)

type subscription struct {
	subject string
	handler func(*nats.Msg)
	queue   string
}

type ActorOption func(*Actor)

// WithSubscription adds a subscription to the Actor for the given subject and handler.
func WithSubscription(subject string, handler func(*nats.Msg)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			subject: subject,
			handler: handler,
			queue:   "",
		})
	}
}

// WithQueueSubscription adds a queue-group subscription to the Actor for the given subject, queue, and handler.
func WithQueueSubscription(subject, queue string, handler func(*nats.Msg)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			subject: subject,
			handler: handler,
			queue:   queue,
		})
	}
}

// Actor represents a NATS client that can publish messages and subscribe to subjects.
type Actor struct {
	id            string
	options       nats.Options
	conn          *nats.Conn
	subscriptions []subscription
	mu            sync.RWMutex
}

// NewActor creates a new Actor with the given name, NATS options, and optional ActorOptions.
func NewActor(id string, options nats.Options, opts ...ActorOption) *Actor {
	a := &Actor{
		id:      id,
		options: options,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Inspect returns the mesh actor spec.
func (a *Actor) Inspect() sup.Spec {
	return sup.Spec{Kind: "mesh"}
}

// Publish sends a message with the given data to the specified subject.
func (a *Actor) Publish(subject string, data []byte) error {
	a.mu.RLock()
	nc := a.conn
	a.mu.RUnlock()

	if nc == nil {
		return errors.New("nats client is not initialized")
	}

	status := nc.Status()
	if status == nats.CLOSED || status == nats.DRAINING_PUBS ||
		status == nats.DRAINING_SUBS {
		return errors.New("nats connection is unusable")
	}

	return nc.Publish(subject, data)
}

// Run starts the Actor's main loop, establishing a NATS connection and setting up subscriptions.
func (a *Actor) Run(ctx context.Context) (err error) {
	defer func() {
		if ctx.Err() != nil {
			err = nil
		}
	}()

	nc, err := a.options.Connect()
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.conn = nc
	a.mu.Unlock()

	for _, subscription := range a.subscriptions {
		if subscription.queue != "" {
			sub, err := nc.QueueSubscribe(subscription.subject, subscription.queue, func(m *nats.Msg) {
				subscription.handler(m)
			})
			if err != nil {
				return err
			}
			defer sub.Unsubscribe()
		} else {
			sub, err := nc.Subscribe(subscription.subject, func(m *nats.Msg) {
				subscription.handler(m)
			})
			if err != nil {
				return err
			}
			defer sub.Unsubscribe()
		}
	}

	<-ctx.Done()

	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.mu.Unlock()

	return nil
}
