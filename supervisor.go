package sup

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrMaxRestartsExceeded is returned when the maximum number of restarts is exceeded within the restart window.
	ErrMaxRestartsExceeded = errors.New("max restarts exceeded")
)

type RestartPolicy uint8

const (
	Permanent RestartPolicy = iota // Always restart, even on clean exits
	Transient                      // Restart on errors/panics, but not on clean exits (nil)
	Temporary                      // Never restart
)

// Option configures a Supervisor.
type SupervisorOption func(*Supervisor)

// WithPolicy sets the restart policy.
func WithPolicy(p RestartPolicy) SupervisorOption {
	return func(s *Supervisor) {
		s.policy = p
	}
}

// WithRestartDelay sets the delay between restarts.
func WithRestartDelay(d time.Duration) SupervisorOption {
	return func(s *Supervisor) {
		s.restartDelay = d
	}
}

// WithRestartLimit sets the maximum number of restarts allowed within a window.
// Both maxRestarts and window must be positive; otherwise NewSupervisor panics.
func WithRestartLimit(maxRestarts int, window time.Duration) SupervisorOption {
	return func(s *Supervisor) {
		s.maxRestarts = maxRestarts
		s.restartWindow = window
	}
}

// WithOnError sets a callback invoked when an actor returns an error or panics.
func WithOnError(fn func(error)) SupervisorOption {
	return func(s *Supervisor) {
		s.onError = fn
	}
}

// WithOnRestart sets a callback invoked just before an actor is restarted.
func WithOnRestart(fn func()) SupervisorOption {
	return func(s *Supervisor) {
		s.onRestart = fn
	}
}

// Supervisor manages the lifecycle of actor Run loops.
type Supervisor struct {
	policy        RestartPolicy
	restartDelay  time.Duration
	maxRestarts   int
	restartWindow time.Duration
	onError       func(error)
	onRestart     func()
	wg            sync.WaitGroup
	running       atomic.Int32
}

// NewSupervisor creates a new Supervisor with the given options.
// Panics if the provided options are invalid.
func NewSupervisor(opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		policy:       Permanent,
		restartDelay: time.Second,
	}

	for _, opt := range opts {
		opt(s)
	}

	if (s.maxRestarts > 0) != (s.restartWindow > 0) {
		panic("sup: WithRestartLimit requires both maxRestarts and window to be positive")
	}

	return s
}

// Go starts the actor's run function in a background goroutine and supervises it.
func (s *Supervisor) Go(ctx context.Context, fn func(context.Context) error) {
	s.running.Add(1)

	s.wg.Go(func() {
		defer s.running.Add(-1)

		var (
			restarts []time.Time
			maxCap   = s.maxRestarts + 1
		)

		if maxCap > 1 {
			restarts = make([]time.Time, 0, maxCap)
		}

		for {
			err := s.executeSafe(ctx, fn)

			if ctx.Err() != nil {
				return
			}

			if err != nil && s.onError != nil {
				s.onError(err)
			}

			if s.policy == Temporary {
				return
			}

			if s.policy == Transient && err == nil {
				return
			}

			if s.maxRestarts > 0 && s.restartWindow > 0 {
				now := time.Now()

				n := 0
				for _, t := range restarts {
					if now.Sub(t) <= s.restartWindow {
						restarts[n] = t
						n++
					}
				}
				restarts = append(restarts[:n], now)

				if len(restarts) > s.maxRestarts {
					if s.onError != nil {
						s.onError(ErrMaxRestartsExceeded)
					}
					return
				}
			}

			delay := s.restartDelay
			if delay == 0 {
				delay = time.Second
			}

			jitterRange := int64(delay) / 10
			if jitterRange > 0 {
				jitter := time.Duration(rand.Int64N(jitterRange))
				if rand.N(2) == 0 {
					delay += jitter
				} else {
					delay -= jitter
				}
			}

			if s.onRestart != nil {
				s.onRestart()
			}

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	})
}

func (s *Supervisor) executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("actor panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return fn(ctx)
}

// Running returns the number of currently running actors under supervision.
func (s *Supervisor) Running() int {
	return int(s.running.Load())
}

// Wait blocks until all actors managed by this supervisor have stopped.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}
