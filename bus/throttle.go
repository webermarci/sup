package bus

import (
	"context"
	"sync"
	"time"

	"github.com/webermarci/sup"
)

// Throttle is a reactive value that limits the rate at which updates from its source
// are broadcast. It ensures that updates are sent at most once per interval,
// always emitting the most recent (trailing) value from that interval.
type Throttle[V any] struct {
	*sup.BaseActor
	broadcaster   broadcaster[V]
	source        Provider[V]
	value         V
	interval      time.Duration
	initialNotify bool
	mu            sync.RWMutex
}

// NewThrottle creates a new Throttle actor attached to a source provider.
func NewThrottle[V any](name string, source Provider[V], interval time.Duration) *Throttle[V] {
	return &Throttle[V]{
		BaseActor:   sup.NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		source:      source,
		interval:    interval,
		// Initialize with the source's current value so Read()
		// returns a valid state before the first reactive update.
		value: source.Read(),
	}
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (t *Throttle[V]) WithSubscriberBuffer(buffer int) *Throttle[V] {
	t.broadcaster.buffer = buffer
	return t
}

// WithInitialNotify configures whether new subscribers receive the current value immediately.
func (t *Throttle[V]) WithInitialNotify(enabled bool) *Throttle[V] {
	t.initialNotify = enabled
	return t
}

// Read returns the currently settled value safely.
func (t *Throttle[V]) Read() V {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.value
}

// Subscribe returns a channel that receives the throttled updates.
func (t *Throttle[V]) Subscribe(ctx context.Context) <-chan V {
	t.mu.RLock()
	current := t.value
	t.mu.RUnlock()
	return t.broadcaster.subscribeValues(ctx, current, t.initialNotify)
}

// Watch returns a channel that receives notifications when the value updates.
func (t *Throttle[V]) Watch(ctx context.Context) <-chan struct{} {
	return t.broadcaster.subscribeNotifications(ctx, t.initialNotify)
}

func (t *Throttle[V]) publish(v V) {
	t.mu.Lock()
	t.value = v
	t.mu.Unlock()
	t.broadcaster.notify(v)
}

// Run is the main actor loop. It manages the trailing-edge rate limit.
func (t *Throttle[V]) Run(ctx context.Context) error {
	in := t.source.Subscribe(ctx)

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
			t.broadcaster.closeAll()
			return nil

		case v, ok := <-in:
			if !ok {
				t.broadcaster.closeAll()
				return nil
			}

			if ready {
				t.publish(v)
				ready = false
				timer.Reset(t.interval)
				timerChan = timer.C
				continue
			}

			pending = v
			havePending = true

		case <-timerChan:
			if havePending {
				t.publish(pending)
				havePending = false
				timer.Reset(t.interval)
				timerChan = timer.C
			} else {
				ready = true
				timerChan = nil
			}
		}
	}
}
