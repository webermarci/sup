package rx_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/webermarci/sup/rx"
)

func TestDebounceEmitsLatestAfterQuietPeriod(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const wait = time.Second
		input := make(chan int)
		output := rx.Debounce(input, wait)

		input <- 1
		synctest.Sleep(wait / 2)
		input <- 2
		synctest.Sleep(wait / 2)
		select {
		case value := <-output:
			t.Fatalf("debounce emitted early value %d", value)
		default:
		}

		input <- 3
		synctest.Sleep(wait)
		if got := receive(t, output); got != 3 {
			t.Fatalf("expected latest value 3, got %d", got)
		}
		close(input)
	})
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
	t.Run("non-positive wait", func(t *testing.T) {
		defer expectPanic(t)
		rx.Debounce(make(chan int), 0)
	})
}
