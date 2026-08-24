package rx

import "time"

// ThrottleFirst emits the first value immediately, then drops input values
// until interval elapses.
func ThrottleFirst[V any](
	input <-chan V,
	interval time.Duration,
) <-chan V {
	if interval <= 0 {
		panic("rx: throttle interval must be positive")
	}

	output := make(chan V, 1)

	go func() {
		defer close(output)

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
				if timerC != nil {
					continue
				}

				sendLatest(output, value)
				if timer == nil {
					timer = time.NewTimer(interval)
				} else {
					timer.Reset(interval)
				}
				timerC = timer.C

			case <-timerC:
				timerC = nil
			}
		}
	}()

	return output
}

// ThrottleLatest emits the latest value received during each interval. A new
// interval begins when the first value arrives after the previous interval.
//
// Closing input stops the timer without flushing a pending value and closes
// the output.
func ThrottleLatest[V any](
	input <-chan V,
	interval time.Duration,
) <-chan V {
	if interval <= 0 {
		panic("rx: throttle interval must be positive")
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
				if timerC != nil {
					continue
				}

				if timer == nil {
					timer = time.NewTimer(interval)
				} else {
					timer.Reset(interval)
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
