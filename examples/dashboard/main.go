package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/httpserver"
	"github.com/webermarci/sup/hub"
	"github.com/webermarci/sup/rx"
)

type Tick struct{}

// TrafficLight is a small state-machine actor. A separate clock sends ticks
// through its typed inbox, and the actor publishes each state as a signal.
type TrafficLight struct {
	ticks *sup.CastInbox[Tick]

	light              *rx.Signal[string]
	secondsRemaining   *rx.Signal[int]
	cyclesCompleted    *rx.Signal[int]
	cycleLengthSeconds *rx.Signal[int]
	paused             *rx.Signal[bool]
}

func NewTrafficLight(
	cycleLengthSeconds *rx.Signal[int],
	paused *rx.Signal[bool],
) *TrafficLight {
	return &TrafficLight{
		ticks:              sup.NewCastInbox[Tick](),
		light:              rx.NewSignal("traffic_light", "green"),
		secondsRemaining:   rx.NewSignal("seconds_remaining", cycleLengthSeconds.Value()),
		cyclesCompleted:    rx.NewSignal("cycles_completed", 0),
		cycleLengthSeconds: cycleLengthSeconds,
		paused:             paused,
	}
}

func (l *TrafficLight) ID() string {
	return "traffic_light_controller"
}

func (l *TrafficLight) Tick(ctx context.Context) error {
	return l.ticks.Cast(ctx, Tick{})
}

func (l *TrafficLight) Run(ctx context.Context) error {
	cycleLengths := l.cycleLengthSeconds.Subscribe(ctx)
	pauses := l.paused.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil

		case paused, ok := <-pauses:
			if !ok {
				return nil
			}
			if paused {
				l.light.Set("yellow")
				l.secondsRemaining.Set(0)
			} else {
				l.light.Set("green")
				l.secondsRemaining.Set(l.cycleLengthSeconds.Value())
			}

		case seconds, ok := <-cycleLengths:
			if !ok {
				return nil
			}
			if seconds < 2 {
				l.cycleLengthSeconds.Set(2)
				continue
			}
			if seconds > 30 {
				l.cycleLengthSeconds.Set(30)
				continue
			}
			if l.light.Value() != "yellow" {
				l.secondsRemaining.Set(seconds)
			}

		case <-l.ticks.Receive():
			if l.paused.Value() {
				continue
			}
			remaining := l.secondsRemaining.Value() - 1
			if remaining > 0 {
				l.secondsRemaining.Set(remaining)
				continue
			}

			switch l.light.Value() {
			case "green":
				l.light.Set("yellow")
				l.secondsRemaining.Set(2)
			case "yellow":
				l.light.Set("red")
				l.secondsRemaining.Set(l.cycleLengthSeconds.Value())
			case "red":
				l.light.Set("green")
				l.secondsRemaining.Set(l.cycleLengthSeconds.Value())
				l.cyclesCompleted.Set(l.cyclesCompleted.Value() + 1)
			}
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

	cycleLengthSeconds := rx.NewSignal("cycle_length_seconds", 5)
	paused := rx.NewSignal("paused", false)
	light := NewTrafficLight(cycleLengthSeconds, paused)

	clock := sup.ActorFunc("clock", func(ctx context.Context) error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if paused.Value() {
					continue
				}
				if err := light.Tick(ctx); err != nil {
					return err
				}
			}
		}
	})

	pedestrianSignal := rx.NewDerived("pedestrian_signal", func() string {
		if paused.Value() {
			return "off"
		}
		if light.light.Value() == "red" {
			return "walk"
		}
		return "wait"
	}, paused, light.light)

	intersection := sup.NewSupervisor("intersection", sup.Transient).
		SetRestartDelay(750*time.Millisecond).
		SetRestartLimit(5, time.Minute).
		AddActors(clock, light)

	dashboard := hub.New("dashboard").
		RegisterActors(clock, light).
		RegisterWritableSignal(cycleLengthSeconds).
		RegisterWritableSignal(paused).
		RegisterSignal(light.light).
		RegisterSignal(light.secondsRemaining).
		RegisterSignal(light.cyclesCompleted).
		RegisterSignal(pedestrianSignal)

	serverActor := httpserver.NewActor("dashboard.http",
		func(context.Context) (*http.Server, error) {
			return &http.Server{Addr: ":8080", Handler: dashboard}, nil
		},
	).SetShutdownTimeout(2 * time.Second)

	root := sup.NewSupervisor("root", sup.Transient).
		AddActors(intersection, pedestrianSignal, dashboard, serverActor)

	go func() {
		if err := root.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "supervision stopped:", err)
			cancel()
		}
	}()

	fmt.Println("traffic dashboard: http://localhost:8080")
	<-ctx.Done()
}
