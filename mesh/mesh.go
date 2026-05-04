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

func WithSubscription(subject string, handler func(*nats.Msg)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			subject: subject,
			handler: handler,
			queue:   "",
		})
	}
}

func WithQueueSubscription(subject, queue string, handler func(*nats.Msg)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			subject: subject,
			handler: handler,
			queue:   queue,
		})
	}
}

type Actor struct {
	*sup.BaseActor
	options       nats.Options
	conn          *nats.Conn
	subscriptions []subscription
	mu            sync.RWMutex
}

func NewActor(name string, options nats.Options, opts ...ActorOption) *Actor {
	a := &Actor{
		BaseActor: sup.NewBaseActor(name),
		options:   options,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

func (a *Actor) Publish(subject string, data []byte) error {
	a.mu.RLock()
	nc := a.conn
	a.mu.RUnlock()

	if nc == nil || !nc.IsConnected() {
		return errors.New("nats client is not connected")
	}
	return nc.Publish(subject, data)
}

func (a *Actor) Run(ctx context.Context) error {
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
