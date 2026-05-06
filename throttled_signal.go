package sup

import (
	"context"
	"sync"
	"time"
)

// ThrottledSignal is a reactive value that limits the rate at which updates from its source
// are broadcast. It ensures that updates are sent at most once per interval,
// always emitting the most recent (trailing) value from that interval.
type ThrottledSignal[V any] struct {
	*BaseActor
	broadcaster   broadcaster[V]
	source        ReadableSignal[V]
	value         V
	interval      time.Duration
	initialNotify bool
	mu            sync.RWMutex
}

// NewThrottledSignal creates a new ThrottledSignal actor attached to a source provider.
func NewThrottledSignal[V any](name string, src ReadableSignal[V], interval time.Duration) *ThrottledSignal[V] {
	return &ThrottledSignal[V]{
		BaseActor:   NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		source:      src,
		interval:    interval,
		// Initialize with the source's current value so Read()
		// returns a valid state before the first reactive update.
		value: src.Read(),
	}
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (s *ThrottledSignal[V]) WithSubscriberBuffer(buffer int) *ThrottledSignal[V] {
	s.broadcaster.buffer = buffer
	return s
}

// WithInitialNotify configures whether new subscribers receive the current value immediately.
func (s *ThrottledSignal[V]) WithInitialNotify(enabled bool) *ThrottledSignal[V] {
	s.initialNotify = enabled
	return s
}

// Read returns the currently settled value safely.
func (s *ThrottledSignal[V]) Read() V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Subscribe returns a channel that receives the throttled updates.
func (s *ThrottledSignal[V]) Subscribe(ctx context.Context) <-chan V {
	s.mu.RLock()
	current := s.value
	s.mu.RUnlock()
	return s.broadcaster.subscribeValues(ctx, current, s.initialNotify)
}

// Watch returns a channel that receives notifications when the value updates.
func (s *ThrottledSignal[V]) Watch(ctx context.Context) <-chan struct{} {
	return s.broadcaster.subscribeNotifications(ctx, s.initialNotify)
}

func (s *ThrottledSignal[V]) publish(v V) {
	s.mu.Lock()
	s.value = v
	s.mu.Unlock()
	s.broadcaster.notify(v)
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
				s.publish(v)
				ready = false
				timer.Reset(s.interval)
				timerChan = timer.C
				continue
			}

			pending = v
			havePending = true

		case <-timerChan:
			if havePending {
				s.publish(pending)
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
