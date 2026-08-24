package sup

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

// RestartPolicy controls when a supervisor restarts a stopped actor.
type RestartPolicy uint8

const (
	// Permanent always restarts actors, even after clean exits.
	Permanent RestartPolicy = iota

	// Transient restarts actors after errors or panics, but not after clean exits.
	Transient

	// Temporary never restarts actors.
	Temporary
)

func (s *Supervisor) runActor(ctx context.Context, actor Actor) error {
	emitSupervisorEvent(ctx, s, EventActorRegistered, actor, nil, 0)
	restarts := 0
	var windowStart time.Time

	for {
		emitSupervisorEvent(ctx, s, EventActorStarted, actor, nil, 0)
		actorCtx, cancelActor := context.WithCancel(ctx)
		err := executeSafe(actorCtx, actor.Run)
		cancelActor()
		emitSupervisorEvent(ctx, s, EventActorStopped, actor, err, 0)

		if ctx.Err() != nil || s.policy == Temporary ||
			(s.policy == Transient && err == nil) {
			return nil
		}

		if s.maxRestarts > 0 {
			now := time.Now()
			if windowStart.IsZero() || now.Sub(windowStart) > s.restartWindow {
				windowStart = now
				restarts = 0
			}
			if restarts >= s.maxRestarts {
				return fmt.Errorf("actor %s exceeded %d restarts in %v",
					actor.ID(), s.maxRestarts, s.restartWindow)
			}
		}

		restarts++
		emitSupervisorEvent(ctx, s, EventActorRestarting, actor, err, restarts)
		if ctx.Err() != nil {
			return nil
		}

		delay := s.restartDelay
		if s.restartDelayFunc != nil {
			delay = s.restartDelayFunc(restarts)
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
		}
	}
}

// Supervisor runs a fixed set of actors and restarts them according to its
// restart policy. A supervisor is itself an Actor and may be nested.
type Supervisor struct {
	id               string
	actors           []Actor
	policy           RestartPolicy
	restartDelay     time.Duration
	restartDelayFunc func(restartCount int) time.Duration
	maxRestarts      int
	restartWindow    time.Duration
	eventHandlers    []EventHandler
}

var _ Actor = (*Supervisor)(nil)

// NewSupervisor creates a supervisor with the given id and restart policy.
// Optional configuration is added with methods before the first Run call.
func NewSupervisor(id string, policy RestartPolicy) *Supervisor {
	if id == "" {
		panic("sup: supervisor id cannot be empty")
	}
	if policy > Temporary {
		panic("sup: invalid restart policy")
	}
	return &Supervisor{
		id:           id,
		policy:       policy,
		restartDelay: time.Second,
	}
}

// AddActors adds actors that start with the supervisor and returns the
// supervisor for chaining. Actors that implement EventHandler observe runtime
// events from this supervisor and its descendants.
func (s *Supervisor) AddActors(actors ...Actor) *Supervisor {
	actorIDs := make(map[string]struct{}, len(s.actors)+len(actors))
	for _, actor := range s.actors {
		actorIDs[actor.ID()] = struct{}{}
	}

	for _, actor := range actors {
		id := actor.ID()
		if id == "" {
			panic("sup: actor id cannot be empty")
		}
		if _, exists := actorIDs[id]; exists {
			panic("sup: duplicate actor id: " + id)
		}
		actorIDs[id] = struct{}{}
	}
	s.actors = append(s.actors, actors...)
	for _, actor := range actors {
		if handler, ok := actor.(EventHandler); ok {
			s.eventHandlers = append(s.eventHandlers, handler)
		}
	}
	return s
}

// SetRestartDelay configures the exact delay between actor restart attempts
// and returns the supervisor for chaining.
func (s *Supervisor) SetRestartDelay(delay time.Duration) *Supervisor {
	if delay < 0 {
		panic("sup: restart delay must be non-negative")
	}
	s.restartDelay = delay
	s.restartDelayFunc = nil
	return s
}

// SetRestartDelayFunc configures a function that calculates the delay before
// each actor restart. The restart count starts at one for the first restart.
// A non-positive result means that the actor should restart immediately. It
// may be called concurrently for different actors and must be safe for
// concurrent use. It returns the supervisor for chaining.
func (s *Supervisor) SetRestartDelayFunc(
	delay func(restartCount int) time.Duration,
) *Supervisor {
	s.restartDelayFunc = delay
	return s
}

// SetRestartLimit limits restarts within a time window and returns the
// supervisor for chaining.
func (s *Supervisor) SetRestartLimit(maxRestarts int, window time.Duration) *Supervisor {
	if maxRestarts <= 0 || window <= 0 {
		panic("sup: maxRestarts and window must be positive")
	}
	s.maxRestarts = maxRestarts
	s.restartWindow = window
	return s
}

// ID returns the supervisor id.
func (s *Supervisor) ID() string { return s.id }

// Run starts all configured actors and blocks until they have all stopped, a
// restart limit is exceeded, or ctx is canceled.
func (s *Supervisor) Run(ctx context.Context) error {
	if len(s.actors) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(withEventHandlers(ctx, s.eventHandlers))
	defer cancel()

	results := make(chan error, len(s.actors))
	for _, actor := range s.actors {
		go func() {
			results <- s.runActor(runCtx, actor)
		}()
	}

	remaining := len(s.actors)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			cancel()
			drain(results, remaining)
			return nil
		case err := <-results:
			remaining--
			if err == nil {
				continue
			}

			cancel()
			drain(results, remaining)
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}

	return nil
}

func drain(results <-chan error, remaining int) {
	for range remaining {
		<-results
	}
}

func executeSafe(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%v\n%s", value, debug.Stack())
		}
	}()

	if err = fn(ctx); ctx.Err() != nil {
		return nil
	}
	return err
}
