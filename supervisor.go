package sup

import (
	"context"
	"errors"
	"fmt"
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

// Supervisor manages the lifecycle of actor Run loops.
type Supervisor struct {
	Policy        RestartPolicy
	RestartDelay  time.Duration
	MaxRestarts   int
	RestartWindow time.Duration
	OnError       func(error)
	OnRestart     func()

	wg      sync.WaitGroup
	running atomic.Int32
}

// Go starts the actor's Run function in a background goroutine and supervises it.
func (s *Supervisor) Go(ctx context.Context, runFn func(context.Context) error) {
	s.running.Add(1)

	s.wg.Go(func() {
		defer s.running.Add(-1)

		var (
			restarts []time.Time
			maxCap   = s.MaxRestarts + 1
		)

		if maxCap > 1 {
			restarts = make([]time.Time, 0, maxCap)
		}

		for {
			err := s.executeSafe(ctx, runFn)

			if ctx.Err() != nil {
				return
			}

			if err != nil && s.OnError != nil {
				s.OnError(err)
			}

			if s.Policy == Temporary {
				return
			}

			if s.Policy == Transient && err == nil {
				return
			}

			if s.MaxRestarts > 0 && s.RestartWindow > 0 {
				now := time.Now()

				n := 0
				for _, t := range restarts {
					if now.Sub(t) <= s.RestartWindow {
						restarts[n] = t
						n++
					}
				}
				restarts = append(restarts[:n], now)

				if len(restarts) > s.MaxRestarts {
					if s.OnError != nil {
						s.OnError(errors.Join(ErrMaxRestartsExceeded, err))
					}
					return
				}
			}

			if s.OnRestart != nil {
				s.OnRestart()
			}

			delay := s.RestartDelay
			if delay == 0 {
				delay = time.Second
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

// executeSafe runs the function and converts panics into standard errors.
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
