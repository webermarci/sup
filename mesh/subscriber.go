package mesh

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go"
	"github.com/webermarci/sup"
)

// Subscriber is an actor responsible for subscribing to a NATS subject and
// handling incoming messages using a provided handler function.
type Subscriber struct {
	*sup.BaseActor
	client  *Client
	subject string
	handler func([]byte)
}

// NewSubscriber creates a new Subscriber actor with the given name, NATS client, subject, and
// message handler function.
func NewSubscriber(name string, client *Client, subject string, handler func([]byte)) *Subscriber {
	return &Subscriber{
		BaseActor: sup.NewBaseActor(name),
		client:    client,
		subject:   subject,
		handler:   handler,
	}
}

// Run starts the Subscriber's main loop, subscribing to the NATS subject and
// processing incoming messages using the handler function.
func (s *Subscriber) Run(ctx context.Context) error {
	nc := s.client.Conn()
	if nc == nil || !nc.IsConnected() {
		return errors.New("nats client is not connected")
	}

	sub, err := nc.Subscribe(s.subject, func(m *nats.Msg) {
		s.handler(m.Data)
	})
	if err != nil {
		return err
	}

	<-ctx.Done()
	return sub.Unsubscribe()
}
