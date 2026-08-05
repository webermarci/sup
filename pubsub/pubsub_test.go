package pubsub_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/sup/pubsub"
)

func TestTopicPublishesToEverySubscription(t *testing.T) {
	ctx := context.Background()
	topic := pubsub.New[int]()
	first := topic.Subscribe(ctx)
	second := topic.Subscribe(ctx)

	published := make(chan error, 1)
	firstReceived := make(chan int, 1)
	secondReceived := make(chan int, 1)
	go func() {
		published <- topic.Publish(ctx, 42)
	}()
	go func() {
		firstReceived <- <-first.Receive()
	}()
	go func() {
		secondReceived <- <-second.Receive()
	}()

	if got := <-firstReceived; got != 42 {
		t.Fatalf("first subscriber received %d, want 42", got)
	}
	if got := <-secondReceived; got != 42 {
		t.Fatalf("second subscriber received %d, want 42", got)
	}
	if err := <-published; err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestPublishWaitsForSubscription(t *testing.T) {
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(context.Background())

	published := make(chan error, 1)
	go func() {
		published <- topic.Publish(context.Background(), 42)
	}()

	select {
	case err := <-published:
		t.Fatalf("publish returned before receive, error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if got := receive(t, subscription); got != 42 {
		t.Fatalf("received %d, want 42", got)
	}
	if err := <-published; err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestSubscriptionClosesWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(ctx)

	if topic.SubscriberCount() != 1 {
		t.Fatalf("subscriber count = %d, want 1", topic.SubscriberCount())
	}

	cancel()

	select {
	case _, ok := <-subscription.Receive():
		if ok {
			t.Fatal("subscription value channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not close after context cancellation")
	}

	if topic.SubscriberCount() != 0 {
		t.Fatalf("subscriber count = %d after cancellation, want 0", topic.SubscriberCount())
	}
}

func TestSubscriptionContextCancellationIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(ctx)

	cancel()
	cancel()

	if _, ok := <-subscription.Receive(); ok {
		t.Fatal("canceled subscription value channel remained open")
	}
	if topic.SubscriberCount() != 0 {
		t.Fatalf("subscriber count = %d, want 0", topic.SubscriberCount())
	}
}

func TestPublishHonorsContextWhenSubscriptionIsNotReceiving(t *testing.T) {
	topic := pubsub.New[int]()
	topic.Subscribe(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := topic.Publish(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("publish error = %v, want deadline exceeded", err)
	}
}

func TestPublishReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	topic := pubsub.New[int]()
	if err := topic.Publish(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error = %v, want canceled", err)
	}
}

func TestConcurrentPublishing(t *testing.T) {
	const (
		publishers    = 8
		valuesPerCopy = 100
	)

	topic := pubsub.New[int]()
	subscription := topic.Subscribe(context.Background())

	const total = publishers * valuesPerCopy
	received := make(chan struct{})
	go func() {
		for range total {
			<-subscription.Receive()
		}
		close(received)
	}()

	var group sync.WaitGroup
	group.Add(publishers)
	for publisher := 0; publisher < publishers; publisher++ {
		go func() {
			defer group.Done()
			for value := 0; value < valuesPerCopy; value++ {
				if err := topic.Publish(context.Background(), value); err != nil {
					t.Errorf("publish returned error: %v", err)
					return
				}
			}
		}()
	}

	group.Wait()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive all published values")
	}
}

func receive[T any](t testing.TB, subscription *pubsub.Subscription[T]) T {
	t.Helper()
	select {
	case value := <-subscription.Receive():
		return value
	case <-time.After(time.Second):
		var zero T
		t.Fatal("timed out waiting for subscription value")
		return zero
	}
}
