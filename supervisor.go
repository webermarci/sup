package sup

import (
	"context"
	"fmt"
	"sync"
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
	OnError       func(error) // Hook to trigger escalation (e.g., One-For-All)

	wg sync.WaitGroup
}

// Go starts the actor's Run function in a background goroutine and supervises it.
func (s *Supervisor) Go(ctx context.Context, runFn func(context.Context) error) {
	s.wg.Go(func() {
		var restarts []time.Time

		for {
			// 1. Execute the actor and catch any panics safely
			err := s.executeSafe(ctx, runFn)

			// 2. If the parent context was canceled, shut down cleanly
			if ctx.Err() != nil {
				return
			}

			// 3. Evaluate the Restart Policy
			if s.Policy == Temporary {
				if err != nil && s.OnError != nil {
					s.OnError(err)
				}
				return
			}

			if s.Policy == Transient && err == nil {
				return // Clean exit, do not restart
			}

			// 4. Evaluate Restart Intensity (MaxRestarts in RestartWindow)
			if s.MaxRestarts > 0 && s.RestartWindow > 0 {
				now := time.Now()
				var recent []time.Time

				// Keep only restarts within the window
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
					return // Give up on this actor
				}
			}

			// 5. Wait before restarting to prevent tight spin loops
			delay := s.RestartDelay
			if delay == 0 {
				delay = time.Second // Default fallback
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				// Loop and restart!
			}
		}
	})
}

// executeSafe runs the function and converts panics into standard errors.
func (s *Supervisor) executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("actor paniced: %v", r)
		}
	}()
	return fn(ctx)
}

// Wait blocks until all actors managed by this supervisor have stopped.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}
