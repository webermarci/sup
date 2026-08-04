package sup

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"
)

// ErrSupervisorRunning is returned when Run is called while a supervisor is
// already running.
var ErrSupervisorRunning = errors.New("sup: supervisor is already running")

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

// RestartDelayFunc calculates the delay before an actor restart. The restart
// count starts at one for the first restart. A non-positive duration means no
// delay.
//
// A supervisor may call the function concurrently for different actors, so it
// must be safe for concurrent use.
type RestartDelayFunc func(restartCount int) time.Duration

// SupervisorOption configures a Supervisor during construction.
type SupervisorOption func(*Supervisor)

// WithActors adds actors that start with the supervisor.
func WithActors(actors ...Actor) SupervisorOption {
	configured := slices.Clone(actors)
	return func(supervisor *Supervisor) {
		actorIDs := make(map[string]struct{}, len(supervisor.actors)+len(configured))
		for _, actor := range supervisor.actors {
			actorIDs[actor.ID()] = struct{}{}
		}

		for _, actor := range configured {
			if actor == nil {
				panic("sup: cannot add nil actor")
			}

			id := actor.ID()
			if id == "" {
				panic("sup: actor id cannot be empty")
			}
			if _, exists := actorIDs[id]; exists {
				panic("sup: duplicate actor id: " + id)
			}
			actorIDs[id] = struct{}{}
		}
		supervisor.actors = append(supervisor.actors, configured...)
	}
}

// WithPolicy configures the supervisor restart policy.
func WithPolicy(policy RestartPolicy) SupervisorOption {
	return func(supervisor *Supervisor) {
		if policy > Temporary {
			panic("sup: invalid restart policy")
		}
		supervisor.policy = policy
	}
}

// WithRestartDelay configures the exact delay between actor restart attempts.
func WithRestartDelay(delay time.Duration) SupervisorOption {
	return func(supervisor *Supervisor) {
		if delay < 0 {
			panic("sup: restart delay must be non-negative")
		}
		supervisor.restartDelay = delay
		supervisor.restartDelayFunc = nil
	}
}

// WithRestartDelayFunc configures a function that calculates the delay before
// each actor restart. The restart count starts at one for the first restart.
// A non-positive result means that the actor should restart immediately.
func WithRestartDelayFunc(delay RestartDelayFunc) SupervisorOption {
	if delay == nil {
		panic("sup: restart delay function cannot be nil")
	}

	return func(supervisor *Supervisor) {
		supervisor.restartDelayFunc = delay
	}
}

// WithRestartLimit limits restarts within a time window.
func WithRestartLimit(maxRestarts int, window time.Duration) SupervisorOption {
	return func(supervisor *Supervisor) {
		if maxRestarts <= 0 || window <= 0 {
			panic("sup: maxRestarts and window must be positive")
		}
		supervisor.maxRestarts = maxRestarts
		supervisor.restartWindow = window
	}
}

// WithEventSink adds an event sink for runtime events from this supervisor and
// its descendants.
func WithEventSink(sinks ...EventSink) SupervisorOption {
	configured := slices.Clone(sinks)
	return func(supervisor *Supervisor) {
		for _, sink := range configured {
			if sink == nil {
				panic("sup: cannot add nil event sink")
			}
		}
		supervisor.eventSinks = append(supervisor.eventSinks, configured...)
	}
}

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
	restartDelayFunc RestartDelayFunc
	maxRestarts      int
	restartWindow    time.Duration
	eventSinks       []EventSink
	running          atomic.Bool
}

var _ Actor = (*Supervisor)(nil)
var _ Inspectable = (*Supervisor)(nil)

// NewSupervisor creates a supervisor with the given id and options.
func NewSupervisor(id string, options ...SupervisorOption) *Supervisor {
	if id == "" {
		panic("sup: supervisor id cannot be empty")
	}
	supervisor := &Supervisor{
		id:           id,
		policy:       Transient,
		restartDelay: time.Second,
	}
	for _, option := range options {
		if option == nil {
			panic("sup: supervisor option cannot be nil")
		}
		option(supervisor)
	}

	return supervisor
}

// ID returns the supervisor id.
func (s *Supervisor) ID() string { return s.id }

// Run starts all configured actors and blocks until they have all stopped, a
// restart limit is exceeded, or ctx is canceled. A supervisor may be run again
// after Run returns. Concurrent calls return ErrSupervisorRunning.
func (s *Supervisor) Run(ctx context.Context) error {
	if ctx == nil {
		panic("sup: supervisor context cannot be nil")
	}
	if !s.running.CompareAndSwap(false, true) {
		return ErrSupervisorRunning
	}
	defer s.running.Store(false)

	if len(s.actors) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(withEventSinks(ctx, s.eventSinks))
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

// Inspect returns the supervisor spec.
func (s *Supervisor) Inspect() Spec {
	return Spec{
		Kind:         "supervisor",
		Dependencies: []string{},
		Metadata: map[string]any{
			"actor_count":    len(s.actors),
			"restart_window": s.restartWindow,
			"restart_delay":  s.restartDelay,
			"max_restarts":   s.maxRestarts,
		},
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
