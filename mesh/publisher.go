package mesh

import (
	"context"
	"errors"

	"github.com/webermarci/sup"
)

type publisherMessage struct {
	data []byte
}

// PublisherOption defines a functional option for configuring a Publisher.
type PublisherOption func(*Publisher)

// WithMailboxSize allows configuring the mailbox size for the Publisher.
func WithMailboxSize(size int) PublisherOption {
	return func(p *Publisher) {
		p.mailbox = sup.NewMailbox(size)
	}
}

// Publisher is an actor responsible for publishing messages to a NATS subject.
type Publisher struct {
	*sup.BaseActor
	mailbox *sup.Mailbox
	client  *Client
	subject string
}

// NewPublisher creates a new Publisher actor with the given name, NATS client, subject, and
// optional configurations.
func NewPublisher(name string, client *Client, subject string, opts ...PublisherOption) *Publisher {
	p := &Publisher{
		BaseActor: sup.NewBaseActor(name),
		mailbox:   sup.NewMailbox(32),
		client:    client,
		subject:   subject,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Publish sends a message to the Publisher's mailbox to be published to the NATS subject.
func (p *Publisher) Publish(data []byte) error {
	_, err := sup.Call[publisherMessage, struct{}](p.mailbox, publisherMessage{data: data})
	return err
}

// Run starts the Publisher's main loop, processing incoming messages and
// publishing them to the NATS subject.
func (p *Publisher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case msg := <-p.mailbox.Receive():
			switch m := msg.(type) {
			case sup.CallRequest[publisherMessage, struct{}]:
				nc := p.client.Conn()
				if nc == nil || !nc.IsConnected() {
					m.Reply(struct{}{}, errors.New("nats client is not connected"))
				}

				if err := nc.Publish(p.subject, m.Payload().data); err != nil {
					m.Reply(struct{}{}, err)
				} else {
					m.Reply(struct{}{}, nil)
				}
			}
		}
	}
}
