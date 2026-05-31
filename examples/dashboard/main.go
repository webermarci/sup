package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/hub"
)

type GetMessage struct{}

type IncrementMessage struct {
	Amount int `json:"amount"`
}

type Counter struct {
	*sup.BaseActor
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
	StateSignal    *sup.Signal[int]
}

func NewCounter(id string) *Counter {
	return &Counter{
		BaseActor:      sup.NewBaseActor(id),
		GetInbox:       sup.NewCallInbox[GetMessage, int](8),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](8),
		StateSignal:    sup.NewSignal(fmt.Sprintf("%s_signal", id), 0),
	}
}

func (c *Counter) Get() int {
	state, _ := c.GetInbox.Call(context.Background(), GetMessage{})
	return state
}

func (c *Counter) Increment(amount int) {
	c.IncrementInbox.Cast(context.Background(), IncrementMessage{Amount: amount})
	c.StateSignal.Write(context.Background(), c.Get())
}

func (c *Counter) Controls() []sup.Control {
	return []sup.Control{
		sup.NewCastControl("increment", c.IncrementInbox),
		sup.NewCallControl("get", c.GetInbox),
	}
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

type CombinedMessage struct {
	Counter         int
	Random          int
	RandomEven      bool
	RandomCharacter string
	Quotient        float64
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	counter := NewCounter("counter")

	random := sup.NewSignal("random", 0).
		Poll(200*time.Millisecond, func(ctx context.Context) (int, error) {
			return rand.IntN(100) + 1, nil
		}).
		Throttle(time.Second)

	isRandomEven := sup.NewDerived("is_random_even", func() bool {
		return random.Read()%2 == 0
	}, random)

	randomCharacter := sup.NewDerived("random_character", func() string {
		return string(rune(random.Read()%26 + 'a'))
	}, random)

	quotient := sup.NewDerived("quotient", func() float64 {
		divisor := random.Read()
		if divisor == 0 {
			return 0
		}
		return float64(counter.StateSignal.Read()) / float64(divisor)
	}, counter.StateSignal, random)

	combined := sup.NewDerived("combined", func() CombinedMessage {
		return CombinedMessage{
			Counter:         counter.StateSignal.Read(),
			Random:          random.Read(),
			RandomEven:      isRandomEven.Read(),
			RandomCharacter: randomCharacter.Read(),
			Quotient:        quotient.Read(),
		}
	}, quotient)

	registry := hub.New("registry",
		hub.WithActor(counter),
		hub.WithSignal(counter.StateSignal),
		hub.WithSignal(random),
		hub.WithSignal(isRandomEven),
		hub.WithSignal(randomCharacter),
		hub.WithSignal(quotient),
		hub.WithSignal(combined),
	)

	root := sup.NewSupervisor("root").
		Observer(registry.Observer()).
		Actors(
			counter,
			counter.StateSignal,
			random,
			isRandomEven,
			randomCharacter,
			quotient,
			combined,
			registry,
		)

	go root.Run(ctx)
	go http.ListenAndServe(":8080", registry.Handler())

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.NewTicker(3 * time.Second).C:
			counter.Increment(1)
		}
	}
}
