package main

import (
	"context"
	"fmt"
	"log/slog"
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
	*sup.BaseActor
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
}

func NewCounter(name string) *Counter {
	return &Counter{
		BaseActor:      sup.NewBaseActor(name),
		GetInbox:       sup.NewCallInbox[GetMessage, int](8),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](8),
	}
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

	supervisor := sup.NewSupervisor("root",
		sup.WithActor(actor),
		sup.WithLogger(slog.Default()),
	)

	go supervisor.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.NewTicker(1 * time.Second).C:
			fmt.Println("current value:", actor.Get())
			actor.Increment(1)
		}
	}
}
