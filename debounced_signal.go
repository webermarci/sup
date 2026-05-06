package sup

import (
	"context"
	"time"
)

// DebouncedSignal is a reactive value that delays broadcasting updates from its source
// until the source has stopped changing for a specified wait duration.
type DebouncedSignal[V any] struct {
	*BaseSignal[V]
	src     ReadableSignal[V]
	wait    time.Duration
	maxWait time.Duration
}

// NewDebouncedSignal creates a new DebouncedSignal actor attached to a source provider.
func NewDebouncedSignal[V any](name string, src ReadableSignal[V], wait time.Duration) *DebouncedSignal[V] {
	s := &DebouncedSignal[V]{
		BaseSignal: NewBaseSignal[V](name),
		src:        src,
		wait:       wait,
	}
	s.value = src.Read()

	return s
}

// SetWait configures the duration to wait after receiving an update from
// the source before broadcasting the new value.
// This allows for coalescing multiple rapid updates into a single update,
// improving efficiency. It acquires a lock to ensure thread-safe access to the wait configuration.
func (s *DebouncedSignal[V]) SetMaxWait(maxWait time.Duration) {
	s.mu.Lock()
	s.maxWait = maxWait
	s.mu.Unlock()
}

// Run is the main actor loop. It subscribes to the source and manages the sliding window.
func (s *DebouncedSignal[V]) Run(ctx context.Context) error {
	in := s.src.Subscribe(ctx)

	var pending V
	var havePending bool
	var burstStart time.Time

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

			now := time.Now()
			pending = v

			if !havePending {
				havePending = true
				burstStart = now
				timer.Reset(s.wait)
				timerChan = timer.C
				continue
			}

			if s.maxWait > 0 && now.Sub(burstStart) >= s.maxWait {
				s.set(pending)
				havePending = false

				timerChan = nil
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			}

			timer.Reset(s.wait)

		case <-timerChan:
			if havePending {
				s.set(pending)
				havePending = false
			}
			timerChan = nil
		}
	}
}
