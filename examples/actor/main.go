package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
)

type GetMessage struct{}

type IncrementMessage struct {
	Amount int
}

type Counter struct {
	id             string
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
}

func NewCounter(id string) *Counter {
	return &Counter{
		id:             id,
		GetInbox:       sup.NewCallInbox[GetMessage, int](),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](),
	}
}

func (c *Counter) ID() string {
	return c.id
}

func (c *Counter) Get() int {
	state, _ := c.GetInbox.Call(context.Background(), GetMessage{})
	return state
}

func (c *Counter) Increment(amount int) {
	c.IncrementInbox.Cast(context.Background(), IncrementMessage{Amount: amount})
}

func (c *Counter) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case message := <-c.GetInbox.Receive():
			message.Reply(c.State, nil)

		case message := <-c.IncrementInbox.Receive():
			c.State += message.Amount
		}
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	actor := NewCounter("counter")

	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(actor)

	go supervisor.Run(ctx)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Println("current value:", actor.Get())
			actor.Increment(1)
		}
	}
}
