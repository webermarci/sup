package rx_test

import (
	"testing"
	"time"

	"github.com/webermarci/sup/rx"
)

func TestDebounceEmitsLatestAfterQuietPeriod(t *testing.T) {
	input := make(chan int)
	output := rx.Debounce(input, 30*time.Millisecond)

	input <- 1
	input <- 2
	input <- 3

	if got := receive(t, output); got != 3 {
		t.Fatalf("expected latest value 3, got %d", got)
	}
	close(input)
}

func TestDebounceDoesNotFlushWhenInputCloses(t *testing.T) {
	input := make(chan int)
	output := rx.Debounce(input, time.Second)

	input <- 1
	close(input)

	if value, ok := <-output; ok {
		t.Fatalf("expected closed output without pending value, got %d", value)
	}
}

func TestDebounceValidatesArguments(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		defer expectPanic(t)
		rx.Debounce((<-chan int)(nil), time.Second)
	})
	t.Run("non-positive wait", func(t *testing.T) {
		defer expectPanic(t)
		rx.Debounce(make(chan int), 0)
	})
}
