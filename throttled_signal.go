package sup

import (
	"context"
	"time"
)

// ThrottledSignal is a reactive value that limits the rate at which updates from its source
// are broadcast. It ensures that updates are sent at most once per interval,
// always emitting the most recent (trailing) value from that interval.
type ThrottledSignal[V any] struct {
	*BaseSignal[V]
	source   ReadableSignal[V]
	interval time.Duration
}

// NewThrottledSignal creates a new ThrottledSignal actor attached to a source provider.
func NewThrottledSignal[V any](name string, src ReadableSignal[V], interval time.Duration) *ThrottledSignal[V] {
	s := &ThrottledSignal[V]{
		BaseSignal: NewBaseSignal[V](name),
		source:     src,
		interval:   interval,
	}
	s.value = src.Read()

	return s
}

// Run is the main actor loop. It manages the trailing-edge rate limit.
func (s *ThrottledSignal[V]) Run(ctx context.Context) error {
	in := s.source.Subscribe(ctx)

	var pending V
	var havePending bool
	ready := true

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	var timerChan <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			s.broadcaster.closeAll()
			return nil

		case v, ok := <-in:
			if !ok {
				s.broadcaster.closeAll()
				return nil
			}

			if ready {
				s.set(v)
				ready = false
				timer.Reset(s.interval)
				timerChan = timer.C
				continue
			}

			pending = v
			havePending = true

		case <-timerChan:
			if havePending {
				s.set(pending)
				havePending = false
				timer.Reset(s.interval)
				timerChan = timer.C
			} else {
				ready = true
				timerChan = nil
			}
		}
	}
}
