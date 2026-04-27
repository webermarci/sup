package sup

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"
)

type RestartPolicy uint8

const (
	Permanent RestartPolicy = iota // Always restart, even on clean exits
	Transient                      // Restart on errors/panics, but not on clean exits (nil)
	Temporary                      // Never restart
)

// SupervisorOption configures a Supervisor.
type SupervisorOption func(*Supervisor)

// WithActor adds an actor to be supervised. Can be called multiple times to add multiple actors.
func WithActor(actor Actor) SupervisorOption {
	return func(s *Supervisor) {
		s.actors = append(s.actors, actor)
	}
}

// WithActors adds multiple actors to be supervised.
func WithActors(actors ...Actor) SupervisorOption {
	return func(s *Supervisor) {
		s.actors = append(s.actors, actors...)
	}
}

// WithPolicy sets the restart policy.
func WithPolicy(policy RestartPolicy) SupervisorOption {
	return func(s *Supervisor) {
		s.policy = policy
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

// WithOnError sets a callback function that will be called whenever a supervised actor returns an error or panics. The callback receives the actor and the error as arguments.
func WithOnError(handler func(actor Actor, err error)) SupervisorOption {
	return func(s *Supervisor) {
		s.onError = handler
	}
}

// Supervisor manages the lifecycle of actor Run loops.
type Supervisor struct {
	*BaseActor
	policy        RestartPolicy
	actors        []Actor
	restartDelay  time.Duration
	maxRestarts   int
	restartWindow time.Duration
	wg            sync.WaitGroup
	onError       func(actor Actor, err error)
	terminalErr   chan error
}

// NewSupervisor creates a new Supervisor with the given options.
// Panics if the provided options are invalid.
func NewSupervisor(name string, opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		BaseActor:    NewBaseActor(name),
		policy:       Transient,
		restartDelay: time.Second,
		terminalErr:  make(chan error, 1),
	}

	for _, opt := range opts {
		opt(s)
	}

	if (s.maxRestarts > 0) != (s.restartWindow > 0) {
		panic("sup: WithRestartLimit requires both maxRestarts and window to be positive")
	}

	return s
}

func (s *Supervisor) executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(
				fmt.Errorf("%v", r),
				fmt.Errorf("%s", debug.Stack()),
			)
		}
	}()
	return fn(ctx)
}

// Spawn starts the given actor under supervision. It will be restarted according to the supervisor's policy if it returns an error or panics.
func (s *Supervisor) Spawn(ctx context.Context, actor Actor) {
	if actor == nil {
		panic("sup: cannot spawn nil actor")
	}

	if actor.Name() == "" {
		panic("sup: actor name cannot be empty")
	}

	s.wg.Go(func() {
		var (
			restarts []time.Time
			maxCap   = s.maxRestarts + 1
		)

		if maxCap > 1 {
			restarts = make([]time.Time, 0, maxCap)
		}

		for {
			err := s.executeSafe(ctx, actor.Run)

			if err != nil && s.onError != nil {
				s.onError(actor, err)
			}

			if ctx.Err() != nil {
				return
			}

			if s.policy == Temporary || (s.policy == Transient && err == nil) {
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
					select {
					case s.terminalErr <- fmt.Errorf("actor %s exceeded max restarts", actor.Name()):
					default:
					}
					return
				}
			}

			delay := s.restartDelay

			jitterRange := int64(delay) / 10
			if jitterRange > 0 {
				jitter := time.Duration(rand.Int64N(jitterRange))
				if rand.N(2) == 0 {
					delay += jitter
				} else {
					delay -= jitter
				}
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

// Run starts all actors under supervision and blocks until the context is canceled or all actors have stopped.
func (s *Supervisor) Run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, actor := range s.actors {
		s.Spawn(childCtx, actor)
	}

	allDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(allDone)
	}()

	select {
	case <-ctx.Done():
		s.wg.Wait()
		return ctx.Err()
	case err := <-s.terminalErr:
		cancel()
		s.wg.Wait()
		return err
	case <-allDone:
		return nil
	}
}

// Wait blocks until all supervised actors have stopped.
func (s *Supervisor) Wait() {
	s.wg.Wait()
}
