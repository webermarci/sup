package rx_test

import (
	"testing"
	"time"

	"github.com/webermarci/sup/rx"
)

func TestThrottleFirstEmitsFirstValueInInterval(t *testing.T) {
	input := make(chan int)
	const interval = 30 * time.Millisecond
	output := rx.ThrottleFirst(input, interval)

	input <- 1
	if got := receive(t, output); got != 1 {
		t.Fatalf("expected first value 1, got %d", got)
	}

	input <- 2
	input <- 3
	select {
	case value := <-output:
		t.Fatalf("unexpected throttled value %d", value)
	default:
	}

	// Leave generous scheduling margin so a busy test runner cannot keep the
	// timer in the previous interval.
	time.Sleep(3 * interval)
	input <- 4
	if got := receive(t, output); got != 4 {
		t.Fatalf("expected next interval value 4, got %d", got)
	}
	close(input)
}

func TestThrottleLatestEmitsLatestValueAtIntervalEnd(t *testing.T) {
	input := make(chan int)
	output := rx.ThrottleLatest(input, 30*time.Millisecond)

	input <- 1
	input <- 2
	input <- 3

	select {
	case value := <-output:
		t.Fatalf("throttle latest emitted early value %d", value)
	default:
	}

	if got := receive(t, output); got != 3 {
		t.Fatalf("expected latest value 3, got %d", got)
	}
	close(input)
}

func TestThrottleLatestDoesNotFlushWhenInputCloses(t *testing.T) {
	input := make(chan int)
	output := rx.ThrottleLatest(input, time.Second)

	input <- 1
	close(input)

	if value, ok := <-output; ok {
		t.Fatalf("expected closed output without pending value, got %d", value)
	}
}

func TestThrottleValidatesArguments(t *testing.T) {
	t.Run("first nil input", func(t *testing.T) {
		defer expectPanic(t)
		rx.ThrottleFirst((<-chan int)(nil), time.Second)
	})
	t.Run("first non-positive interval", func(t *testing.T) {
		defer expectPanic(t)
		rx.ThrottleFirst(make(chan int), 0)
	})
	t.Run("latest nil input", func(t *testing.T) {
		defer expectPanic(t)
		rx.ThrottleLatest((<-chan int)(nil), time.Second)
	})
	t.Run("latest non-positive interval", func(t *testing.T) {
		defer expectPanic(t)
		rx.ThrottleLatest(make(chan int), 0)
	})
}
