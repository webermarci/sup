package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/control"
	"github.com/webermarci/sup/ui"
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
	StateSignal    *sup.PushedSignal[int]
}

func NewCounter(id string) *Counter {
	return &Counter{
		BaseActor:      sup.NewBaseActor(id),
		GetInbox:       sup.NewCallInbox[GetMessage, int](8),
		IncrementInbox: sup.NewCastInbox[IncrementMessage](8),
		StateSignal: sup.NewPushedSignal(fmt.Sprintf("%s_signal", id), func(ctx context.Context, i int) error {
			return nil
		}),
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
	Counter                  int
	ThrottledRandom          int
	ThrottledRandomEven      bool
	ThrottledRandomCharacter string
	Quotient                 float64
}

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	counter := NewCounter("counter")

	random := sup.NewPeriodicSignal("random", func(ctx context.Context) (int, error) {
		return rand.IntN(100) + 1, nil
	}, 100*time.Millisecond)

	throttledRandom := sup.NewThrottledSignal("throttled_random", random, time.Second)

	isThrottledRandomEven := sup.NewComputedSignal("is_throttled_random_even", func() bool {
		return throttledRandom.Read()%2 == 0
	}, throttledRandom)

	throttledRandomCharacter := sup.NewComputedSignal("throttled_random_character", func() string {
		return string(rune(throttledRandom.Read()%26 + 'a'))
	}, throttledRandom)

	quotient := sup.NewComputedSignal("quotient", func() float64 {
		divisor := throttledRandom.Read()
		if divisor == 0 {
			return 0
		}
		return float64(counter.StateSignal.Read()) / float64(divisor)
	}, counter.StateSignal, throttledRandom)

	combined := sup.NewComputedSignal("combined", func() CombinedMessage {
		return CombinedMessage{
			Counter:                  counter.StateSignal.Read(),
			ThrottledRandom:          throttledRandom.Read(),
			ThrottledRandomEven:      isThrottledRandomEven.Read(),
			ThrottledRandomCharacter: throttledRandomCharacter.Read(),
			Quotient:                 quotient.Read(),
		}
	}, quotient)

	registry := control.NewRegistry("registry",
		control.WithActor(counter,
			control.Cast("increment", func(ctx context.Context, input IncrementMessage) error {
				counter.Increment(input.Amount)
				return nil
			}),
			control.Call("get", func(ctx context.Context, input GetMessage) (int, error) {
				return counter.Get(), nil
			}),
		),
		control.WithSignal(counter.StateSignal),
		control.WithSignal(random),
		control.WithSignal(throttledRandom),
		control.WithSignal(isThrottledRandomEven),
		control.WithSignal(throttledRandomCharacter),
		control.WithSignal(quotient),
		control.WithSignal(combined),
	)

	root := sup.NewSupervisor("root",
		sup.WithActors(
			counter,
			counter.StateSignal,
			random,
			throttledRandom,
			isThrottledRandomEven,
			throttledRandomCharacter,
			quotient,
			combined,
			registry,
		),
		sup.WithLogger(slog.Default()),
	)

	dashboard := ui.NewDashboard(registry)

	go root.Run(ctx)
	go http.ListenAndServe(":8080", dashboard.Handler())

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.NewTicker(3 * time.Second).C:
			counter.Increment(1)
		}
	}
}
