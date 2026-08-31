package sup_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/webermarci/sup"
)

func ExampleActorFunc() {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})

	worker := sup.ActorFunc("worker", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(worker)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	<-started
	fmt.Println(worker.ID(), "started")
	cancel()
	fmt.Println(<-done)

	// Output:
	// worker started
	// <nil>
}

func ExampleCastInbox() {
	type increment struct {
		Amount int
	}

	ctx, cancel := context.WithCancel(context.Background())
	inbox := sup.NewCastInbox[increment]()
	processed := make(chan int, 1)

	worker := sup.ActorFunc("counter", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case message := <-inbox.Receive():
			processed <- message.Amount
			return nil
		}
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(worker)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	if err := inbox.Cast(ctx, increment{Amount: 42}); err != nil {
		panic(err)
	}
	fmt.Println(<-processed)

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}

	// Output:
	// 42
}

func ExampleCallInbox() {
	type multiply struct {
		Left  int
		Right int
	}

	ctx, cancel := context.WithCancel(context.Background())
	inbox := sup.NewCallInbox[multiply, int]()

	worker := sup.ActorFunc("multiplier", func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case request := <-inbox.Receive():
				result := request.Payload().Left * request.Payload().Right
				if !request.Reply(result, nil) {
					continue
				}
			}
		}
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(worker)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	result, err := inbox.Call(ctx, multiply{Left: 6, Right: 7})
	if err != nil {
		panic(err)
	}
	fmt.Println(result)

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}

	// Output:
	// 42
}

func ExampleSupervisor() {
	var attempts atomic.Int32
	worker := sup.ActorFunc("worker", func(context.Context) error {
		if attempts.Add(1) == 1 {
			return errors.New("temporary failure")
		}
		return nil
	})

	supervisor := sup.NewSupervisor("root", sup.Transient).
		SetRestartDelay(0).
		AddActors(worker)

	err := supervisor.Run(context.Background())
	fmt.Println("attempts:", attempts.Load())
	fmt.Println("error:", err)

	// Output:
	// attempts: 2
	// error: <nil>
}
