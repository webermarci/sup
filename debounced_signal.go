package sup

import (
	"context"
	"sync"
	"time"
)

// DebouncedSignal is a reactive value that delays broadcasting updates from its source
// until the source has stopped changing for a specified wait duration.
type DebouncedSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	src           ReadableSignal[V]
	value         V
	wait          time.Duration
	maxWait       time.Duration
	initialNotify bool
	mu            sync.RWMutex
}

// NewDebouncedSignal creates a new DebouncedSignal actor attached to a source provider.
func NewDebouncedSignal[V any](name string, src ReadableSignal[V], wait time.Duration) *DebouncedSignal[V] {
	return &DebouncedSignal[V]{
		BaseActor:   NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		src:         src,
		wait:        wait,
		value:       src.Read(),
	}
}

// WithMaxWait configures a maximum time to wait before forcing an update,
// preventing infinite starvation if the source is constantly changing.
func (s *DebouncedSignal[V]) WithMaxWait(maxWait time.Duration) *DebouncedSignal[V] {
	s.maxWait = maxWait
	return s
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (s *DebouncedSignal[V]) WithSubscriberBuffer(buffer int) *DebouncedSignal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers receive the current value immediately.
func (s *DebouncedSignal[V]) WithInitialNotify(enabled bool) *DebouncedSignal[V] {
	s.initialNotify = enabled
	return s
}

// Read returns the currently settled value safely.
func (s *DebouncedSignal[V]) Read() V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Subscribe returns a channel that receives the debounced updates.
func (s *DebouncedSignal[V]) Subscribe(ctx context.Context) <-chan V {
	s.mu.RLock()
	current := s.value
	s.mu.RUnlock()
	return s.broadcaster.subscribeValues(ctx, current, s.initialNotify)
}

// Watch returns a channel that receives notifications when the value settles.
func (s *DebouncedSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	return s.broadcaster.subscribeNotifications(ctx, s.initialNotify)
}

func (s *DebouncedSignal[V]) publish(v V) {
	s.mu.Lock()
	s.value = v
	s.mu.Unlock()
	s.broadcaster.notify(v)
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
				s.publish(pending)
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
				s.publish(pending)
				havePending = false
			}
			timerChan = nil
		}
	}
}
