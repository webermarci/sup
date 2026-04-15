package sup

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
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

	wg      sync.WaitGroup
	running atomic.Int32
}

// Go starts the actor's Run function in a background goroutine and supervises it.
func (s *Supervisor) Go(ctx context.Context, runFn func(context.Context) error) {
	s.wg.Go(func() {
		s.running.Add(1)
		defer s.running.Add(-1)

		var restarts []time.Time

		for {
			err := s.executeSafe(ctx, runFn)

			if ctx.Err() != nil {
				return
			}

			if s.Policy == Temporary {
				if err != nil && s.OnError != nil {
					s.OnError(err)
				}
				return
			}

			if s.Policy == Transient && err == nil {
				return
			}

			if s.MaxRestarts > 0 && s.RestartWindow > 0 {
				now := time.Now()
				var recent []time.Time

				for _, t := range restarts {
					if now.Sub(t) <= s.RestartWindow {
						recent = append(recent, t)
					}
				}
				recent = append(recent, now)
				restarts = recent

				if len(restarts) > s.MaxRestarts {
					if s.OnError != nil {
						s.OnError(fmt.Errorf("max restarts exceeded: %v", err))
					}
					return
				}
			}

			delay := s.RestartDelay
			if delay == 0 {
				delay = time.Second
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	})
}

// executeSafe runs the function and converts panics into standard errors.
func (s *Supervisor) executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("actor panicked: %v", r)
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
