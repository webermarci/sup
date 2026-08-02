package rx

import "time"

// Debounce emits the latest value after no input arrives for wait.
//
// Closing input stops the timer without flushing a pending value and closes
// the output.
func Debounce[V any](input <-chan V, wait time.Duration) <-chan V {
	if input == nil {
		panic("rx: debounce input cannot be nil")
	}

	if wait <= 0 {
		panic("rx: debounce wait must be positive")
	}

	output := make(chan V, 1)

	go func() {
		defer close(output)

		var latest V
		var timer *time.Timer
		var timerC <-chan time.Time

		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()

		for {
			select {
			case value, ok := <-input:
				if !ok {
					return
				}

				latest = value
				if timer == nil {
					timer = time.NewTimer(wait)
				} else {
					timer.Reset(wait)
				}
				timerC = timer.C

			case <-timerC:
				timerC = nil
				sendLatest(output, latest)
			}
		}
	}()

	return output
}
