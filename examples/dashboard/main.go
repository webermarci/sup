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
	"github.com/webermarci/sup/rx"
)

type GetMessage struct{}

type IncrementMessage struct {
	Amount int `json:"amount"`
}

type Counter struct {
	id             string
	GetInbox       *sup.CallInbox[GetMessage, int]
	IncrementInbox *sup.CastInbox[IncrementMessage]
	State          int
	StateSignal    *rx.Signal[int]
	EnabledSignal  *rx.Signal[bool]
}

func NewCounter(id string) *Counter {
	return &Counter{
		id:             id,
		GetInbox:       sup.NewCallInbox[GetMessage, int](8),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](8),
		StateSignal:    rx.NewSignal(fmt.Sprintf("%s_signal", id), 0),
		EnabledSignal:  rx.NewSignal(fmt.Sprintf("%s_enabled", id), true),
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
	enabledValues := c.EnabledSignal.Subscribe(ctx)
	enabled := c.EnabledSignal.Value()

	for {
		select {
		case <-ctx.Done():
			return nil

		case value, ok := <-enabledValues:
			if !ok {
				return nil
			}
			enabled = value

		case message := <-c.GetInbox.Receive():
			message.Reply(c.State, nil)

		case message := <-c.IncrementInbox.Receive():
			if !enabled {
				continue
			}
			c.State += message.Amount
			c.StateSignal.Set(c.State)
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

	rawRandom := rx.NewSignal("raw_random", 0)
	random := rx.NewSignal("random", 0)

	randomPoller := sup.ActorFunc("random_poller", func(ctx context.Context) error {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rawRandom.Set(rand.IntN(100) + 1)
			case <-ctx.Done():
				return nil
			}
		}
	})

	randomThrottler := sup.ActorFunc("random_throttler", func(ctx context.Context) error {
		for value := range rx.ThrottleLatest(rawRandom.Subscribe(ctx), time.Second) {
			random.Set(value)
		}
		return nil
	})

	isRandomEven := rx.NewDerived("is_random_even", func() bool {
		return random.Value()%2 == 0
	}, random)

	randomCharacter := rx.NewDerived("random_character", func() string {
		return string(rune(random.Value()%26 + 'a'))
	}, random)

	quotient := rx.NewDerived("quotient", func() float64 {
		divisor := random.Value()
		if divisor == 0 {
			return 0
		}
		return float64(counter.StateSignal.Value()) / float64(divisor)
	}, counter.StateSignal, random)

	combined := rx.NewDerived("combined", func() CombinedMessage {
		return CombinedMessage{
			Counter:         counter.StateSignal.Value(),
			Random:          random.Value(),
			RandomEven:      isRandomEven.Value(),
			RandomCharacter: randomCharacter.Value(),
			Quotient:        quotient.Value(),
		}
	}, counter.StateSignal, random, isRandomEven, randomCharacter, quotient)

	dashboard := hub.New("dashboard",
		hub.WithActor(counter),
		hub.WithSignal(counter.StateSignal),
		hub.WithWritableSignal(counter.EnabledSignal),
		hub.WithSignal(random),
		hub.WithSignal(isRandomEven),
		hub.WithSignal(randomCharacter),
		hub.WithSignal(quotient),
		hub.WithSignal(combined),
	)

	root := sup.NewSupervisor("root",
		sup.WithEventSink(dashboard),
		sup.WithActors(
			counter,
			randomPoller,
			randomThrottler,
			isRandomEven,
			randomCharacter,
			quotient,
			combined,
			dashboard,
		),
	)

	go root.Run(ctx)
	go http.ListenAndServe(":8080", dashboard.Handler())

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			counter.Increment(1)
		}
	}
}
