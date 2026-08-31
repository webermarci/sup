package rx_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/rx"
)

func ExampleSignal() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	temperature := rx.NewSignal("temperature", 20)
	values := temperature.Subscribe(ctx)

	fmt.Println(<-values)
	temperature.Set(21)
	fmt.Println(<-values)

	// Output:
	// 20
	// 21
}

func ExampleSignal_Update() {
	count := rx.NewSignal("count", 1)
	count.Update(func(current int) int {
		return current + 1
	})

	fmt.Println(count.Value())

	// Output:
	// 2
}

func ExampleSignal_Watch() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status := rx.NewSignal("status", "starting")
	changes := status.Watch(ctx)
	<-changes // Watch starts with one notification.

	status.Set("ready")
	<-changes
	fmt.Println(status.Value())

	// Output:
	// ready
}

func ExampleDerived() {
	ctx, cancel := context.WithCancel(context.Background())
	left := rx.NewSignal("left", 2)
	right := rx.NewSignal("right", 3)
	sum := rx.NewDerived("sum", func() int {
		return left.Value() + right.Value()
	}, left, right)

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(sum)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	left.Set(4)
	value, err := rx.WaitFor(ctx, sum, func(value int) bool {
		return value == 7
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(value)

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}

	// Output:
	// 7
}

func ExampleMap() {
	numbers := make(chan int)
	labels := rx.Map(numbers, func(value int) string {
		return fmt.Sprintf("item-%d", value)
	})

	numbers <- 2
	fmt.Println(<-labels)
	close(numbers)

	// Output:
	// item-2
}

func ExampleFilter() {
	numbers := make(chan int)
	even := rx.Filter(numbers, func(value int) bool {
		return value%2 == 0
	})

	numbers <- 1
	numbers <- 2
	fmt.Println(<-even)
	close(numbers)

	// Output:
	// 2
}

func ExampleDistinct() {
	input := make(chan int)
	unique := rx.Distinct(input)

	input <- 1
	fmt.Println(<-unique)
	input <- 1 // Repeated values are skipped.
	input <- 2
	fmt.Println(<-unique)
	close(input)

	// Output:
	// 1
	// 2
}

func ExampleDistinctFunc() {
	input := make(chan string)
	unique := rx.DistinctFunc(input, strings.EqualFold)

	input <- "READY"
	fmt.Println(<-unique)
	input <- "ready" // Equal according to strings.EqualFold.
	input <- "done"
	fmt.Println(<-unique)
	close(input)

	// Output:
	// READY
	// done
}

func ExampleTransitions() {
	states := make(chan string)
	changes := rx.Transitions(states)

	states <- "idle"
	states <- "running"
	change := <-changes
	fmt.Printf("%s -> %s\n", change.From, change.To)
	close(states)

	// Output:
	// idle -> running
}

func ExampleTransitionsFunc() {
	states := make(chan string)
	changes := rx.TransitionsFunc(states, strings.EqualFold)

	states <- "IDLE"
	states <- "idle" // Equal according to strings.EqualFold.
	states <- "running"
	change := <-changes
	fmt.Printf("%s -> %s\n", change.From, change.To)
	close(states)

	// Output:
	// IDLE -> running
}

func ExampleEdges() {
	input := make(chan bool)
	edges := rx.Edges(input)

	input <- false
	input <- true
	fmt.Println(<-edges)
	input <- false
	fmt.Println(<-edges)
	close(input)

	// Output:
	// rising
	// falling
}

func ExampleDebounce() {
	input := make(chan string)
	settled := rx.Debounce(input, time.Millisecond)

	input <- "search query"
	fmt.Println(<-settled)
	close(input)

	// Output:
	// search query
}

func ExampleThrottleFirst() {
	input := make(chan int)
	first := rx.ThrottleFirst(input, time.Second)

	input <- 1
	fmt.Println(<-first)
	close(input)

	// Output:
	// 1
}

func ExampleThrottleLatest() {
	input := make(chan int)
	latest := rx.ThrottleLatest(input, time.Millisecond)

	input <- 3
	fmt.Println(<-latest)
	close(input)

	// Output:
	// 3
}

func ExampleWaitFor() {
	status := rx.NewSignal("status", "starting")
	status.Set("ready")

	value, err := rx.WaitFor(context.Background(), status, func(value string) bool {
		return value == "ready"
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(value)

	// Output:
	// ready
}
