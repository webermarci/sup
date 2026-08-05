// Package pubsub provides typed, in-memory topics for one-to-many
// communication.
//
// # Getting started
//
// Create a Topic, subscribe with a context, and publish values to every active
// subscriber:
//
//	topic := pubsub.New[OrderCreated]()
//	subscription := topic.Subscribe(ctx)
//
//	go func() {
//		for event := range subscription.Receive() {
//			handle(event)
//		}
//	}()
//
//	if err := topic.Publish(ctx, OrderCreated{ID: "order_123"}); err != nil {
//		// handle cancellation or an unavailable subscriber
//	}
//
// A Topic is passive and does not need to be started or supervised. It can be
// shared by ordinary goroutines and sup actors.
//
// # Semantics
//
// Subscriptions are unbuffered. Publish blocks until every subscription that
// was active when publishing began receives the value, so a subscriber that is
// not receiving applies direct backpressure. Publish accepts a context to stop
// waiting. Delivery that already reached a subscriber cannot be rolled back.
// Publishing to a topic with no subscribers returns immediately.
//
// Receive returns a read-only channel that can be ranged over or used in a
// select. A handler that performs slow I/O before receiving the next value also
// applies backpressure. When decoupling is required, consume the subscription
// in a goroutine and compose it with an application-owned worker or queue.
//
// A subscription's context controls its lifetime. Canceling the context removes
// the subscription and closes its receive channel. Subscriptions do not have a
// separate Close method. Topics do not persist values, replay missed
// publications, or retry failed handling; work that needs those guarantees
// belongs in an explicit worker or durable queue.
//
// See the README for application examples and the package reference at
// https://pkg.go.dev/github.com/webermarci/sup/pubsub for the complete API.
package pubsub
