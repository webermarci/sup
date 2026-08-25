package main

import (
	"context"
	"fmt"
	"time"

	"github.com/webermarci/sup/rx"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	motorState := rx.NewSignal("motor_state", "stopped")
	transitions := rx.Transitions(motorState.Subscribe(ctx))

	for _, state := range []string{"starting", "running"} {
		time.Sleep(100 * time.Millisecond)
		motorState.Set(state)

		transition := <-transitions
		fmt.Printf("Motor: %s -> %s\n", transition.From, transition.To)
	}
}
