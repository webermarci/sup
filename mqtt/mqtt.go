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

func WithSubscription(topic string, qos byte, handler func(mqtt.Message)) ActorOption {
	return func(a *Actor) {
		a.subscriptions = append(a.subscriptions, subscription{
			topic:   topic,
			handler: handler,
			qos:     qos,
		})
	}
}

type Actor struct {
	*sup.BaseActor
	options       *mqtt.ClientOptions
	subscriptions []subscription
	client        mqtt.Client
	mu            sync.RWMutex
}

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
