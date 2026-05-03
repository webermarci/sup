package bus

import (
	"context"
	"sync"
	"time"

	"github.com/webermarci/sup"
)

// Debounce is a reactive value that delays broadcasting updates from its source
// until the source has stopped changing for a specified wait duration.
type Debounce[V any] struct {
	*sup.BaseActor
	broadcaster   broadcaster[V]
	src           Provider[V]
	value         V
	wait          time.Duration
	maxWait       time.Duration
	initialNotify bool
	mu            sync.RWMutex
}

// NewDebounce creates a new Debounce actor attached to a source provider.
func NewDebounce[V any](name string, src Provider[V], wait time.Duration) *Debounce[V] {
	return &Debounce[V]{
		BaseActor:   sup.NewBaseActor(name),
		broadcaster: broadcaster[V]{buffer: 16},
		src:         src,
		wait:        wait,
		value:       src.Read(),
	}
}

// WithMaxWait configures a maximum time to wait before forcing an update,
// preventing infinite starvation if the source is constantly changing.
func (d *Debounce[V]) WithMaxWait(maxWait time.Duration) *Debounce[V] {
	d.maxWait = maxWait
	return d
}

// WithSubscriberBuffer configures the buffer size for subscriber channels.
func (d *Debounce[V]) WithSubscriberBuffer(buffer int) *Debounce[V] {
	d.broadcaster.buffer = buffer
	return d
}

// WithInitialNotify configures whether new subscribers receive the current value immediately.
func (d *Debounce[V]) WithInitialNotify(enabled bool) *Debounce[V] {
	d.initialNotify = enabled
	return d
}

// Read returns the currently settled value safely.
func (d *Debounce[V]) Read() V {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.value
}

// Subscribe returns a channel that receives the debounced updates.
func (d *Debounce[V]) Subscribe(ctx context.Context) <-chan V {
	d.mu.RLock()
	current := d.value
	d.mu.RUnlock()
	return d.broadcaster.subscribeValues(ctx, current, d.initialNotify)
}

// Watch returns a channel that receives notifications when the value settles.
func (d *Debounce[V]) Watch(ctx context.Context) <-chan struct{} {
	return d.broadcaster.subscribeNotifications(ctx, d.initialNotify)
}

func (d *Debounce[V]) publish(v V) {
	d.mu.Lock()
	d.value = v
	d.mu.Unlock()
	d.broadcaster.notify(v)
}

// Run is the main actor loop. It subscribes to the source and manages the sliding window.
func (d *Debounce[V]) Run(ctx context.Context) error {
	in := d.src.Subscribe(ctx)

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
			d.broadcaster.closeAll()
			return nil

		case v, ok := <-in:
			if !ok {
				d.broadcaster.closeAll()
				return nil
			}

			now := time.Now()
			pending = v

			if !havePending {
				havePending = true
				burstStart = now
				timer.Reset(d.wait)
				timerChan = timer.C
				continue
			}

			if d.maxWait > 0 && now.Sub(burstStart) >= d.maxWait {
				d.publish(pending)
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

			timer.Reset(d.wait)

		case <-timerChan:
			if havePending {
				d.publish(pending)
				havePending = false
			}
			timerChan = nil
		}
	}
}
