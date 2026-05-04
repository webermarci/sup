package mqtt

import (
	"context"
	"errors"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/webermarci/sup"
)

type subscription struct {
	topic   string
	handler func(mqtt.Message)
	qos     byte
}

type ActorOption func(*Actor)

// WithSubscription adds a subscription to the Actor for the given topic, QoS, and handler.
func WithSubscription(topic string, qos byte, handler func(mqtt.Message)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			topic:   topic,
			handler: handler,
			qos:     qos,
		})
	}
}

// Actor represents an MQTT client that can publish messages and subscribe to topics.
type Actor struct {
	*sup.BaseActor
	options       *mqtt.ClientOptions
	subscriptions []subscription
	client        mqtt.Client
	mu            sync.RWMutex
}

// NewActor creates a new Actor with the given name, MQTT client options, and optional ActorOptions.
func NewActor(name string, options *mqtt.ClientOptions, opts ...ActorOption) *Actor {
	a := &Actor{
		BaseActor: sup.NewBaseActor(name),
		options:   options,
	}

	for _, o := range opts {
		o(a)
	}

	options.SetAutoReconnect(false)

	return a
}

// Publish sends a message to the specified topic with the given QoS and retained flag.
func (a *Actor) Publish(topic string, qos byte, retained bool, payload any) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return mqtt.ErrNotConnected
	}

	token := client.Publish(topic, qos, retained, payload)
	if !token.WaitTimeout(2 * time.Second) {
		return errors.New("mqtt publish timeout")
	}
	return token.Error()
}

// Run starts the Actor, connecting to the MQTT broker and subscribing to topics. It blocks until the context is canceled or a connection error occurs.
func (a *Actor) Run(ctx context.Context) error {
	errChan := make(chan error, 1)

	opts := *a.options

	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		select {
		case errChan <- err:
		default:
		}
	})

	newClient := mqtt.NewClient(&opts)

	a.mu.Lock()
	a.client = newClient
	a.mu.Unlock()

	if token := a.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	for _, sub := range a.subscriptions {
		if token := a.client.Subscribe(sub.topic, sub.qos, func(client mqtt.Client, msg mqtt.Message) {
			sub.handler(msg)
		}); token.Wait() && token.Error() != nil {
			return token.Error()
		}
	}

	select {
	case <-ctx.Done():
		a.client.Disconnect(250)
		return nil
	case err := <-errChan:
		return err
	}
}
