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

type supervisorConfig struct {
	policy        RestartPolicy
	restartDelay  time.Duration
	maxRestarts   int
	restartWindow time.Duration
	eventSinks    []EventSink
}

// SupervisorOption configures a Supervisor during construction.
type SupervisorOption func(*Supervisor)

// WithActors adds actors that start with the supervisor.
func WithActors(actors ...Actor) SupervisorOption {
	configured := slices.Clone(actors)
	return func(supervisor *Supervisor) {
		supervisor.actors = append(supervisor.actors, configured...)
	}
}

// WithPolicy configures the supervisor restart policy.
func WithPolicy(policy RestartPolicy) SupervisorOption {
	validateRestartPolicy(policy)
	return func(supervisor *Supervisor) { supervisor.config.policy = policy }
}

// WithRestartDelay configures the exact delay between actor restart attempts.
func WithRestartDelay(delay time.Duration) SupervisorOption {
	validateRestartDelay(delay)
	return func(supervisor *Supervisor) { supervisor.config.restartDelay = delay }
}

// WithRestartLimit limits restarts within a time window.
func WithRestartLimit(maxRestarts int, window time.Duration) SupervisorOption {
	validateRestartLimit(maxRestarts, window)
	return func(supervisor *Supervisor) {
		supervisor.config.maxRestarts = maxRestarts
		supervisor.config.restartWindow = window
	}
}

// WithEventSink adds an event sink for runtime events from this supervisor and
// its descendants.
func WithEventSink(sinks ...EventSink) SupervisorOption {
	configured := slices.Clone(sinks)
	return func(supervisor *Supervisor) {
		supervisor.config.eventSinks = append(supervisor.config.eventSinks, configured...)
	}
}

func validateRestartPolicy(policy RestartPolicy) {
	if policy > Temporary {
		panic("sup: invalid restart policy")
	}
}

func validateRestartDelay(delay time.Duration) {
	if delay < 0 {
		panic("sup: restart delay must be non-negative")
	}
}

func validateRestartLimit(maxRestarts int, window time.Duration) {
	if maxRestarts <= 0 || window <= 0 {
		panic("sup: maxRestarts and window must be positive")
	}
}

func defaultSupervisorConfig() supervisorConfig {
	return supervisorConfig{
		policy:       Transient,
		restartDelay: time.Second,
	}
}

func validateEventSinks(sinks []EventSink) {
	for _, sink := range sinks {
		if sink == nil {
			panic("sup: cannot add nil event sink")
		}
	}
}

func validateActor(actor Actor) {
	if actor == nil {
		panic("sup: cannot add nil actor")
	}
	if actor.ID() == "" {
		panic("sup: actor id cannot be empty")
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

		if ctx.Err() != nil || s.config.policy == Temporary ||
			(s.config.policy == Transient && err == nil) {
			return nil
		}

		if s.config.maxRestarts > 0 {
			now := time.Now()
			if windowStart.IsZero() || now.Sub(windowStart) > s.config.restartWindow {
				windowStart = now
				restarts = 0
			}
			if restarts >= s.config.maxRestarts {
				return fmt.Errorf("actor %s exceeded %d restarts in %v",
					actor.ID(), s.config.maxRestarts, s.config.restartWindow)
			}
		}

		restarts++
		emitSupervisorEvent(ctx, s, EventActorRestarting, actor, err, restarts)
		if ctx.Err() != nil {
			return nil
		}
		if s.config.restartDelay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(s.config.restartDelay):
			}
		}
	}
}

// Supervisor runs a fixed set of actors and restarts them according to its
// restart policy. A supervisor is itself an Actor and may be nested.
type Supervisor struct {
	id      string
	actors  []Actor
	config  supervisorConfig
	running atomic.Bool
}

var _ Actor = (*Supervisor)(nil)
var _ Inspectable = (*Supervisor)(nil)

// NewSupervisor creates a supervisor with the given id and options.
func NewSupervisor(id string, options ...SupervisorOption) *Supervisor {
	if id == "" {
		panic("sup: supervisor id cannot be empty")
	}
	supervisor := &Supervisor{
		id:     id,
		config: defaultSupervisorConfig(),
	}
	for _, option := range options {
		if option == nil {
			panic("sup: supervisor option cannot be nil")
		}
		option(supervisor)
	}

	actorIDs := make(map[string]struct{}, len(supervisor.actors))
	for _, actor := range supervisor.actors {
		validateActor(actor)
		if _, exists := actorIDs[actor.ID()]; exists {
			panic("sup: duplicate actor id: " + actor.ID())
		}
		actorIDs[actor.ID()] = struct{}{}
	}
	validateEventSinks(supervisor.config.eventSinks)
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

	runCtx, cancel := context.WithCancel(withEventSinks(ctx, s.config.eventSinks))
	defer cancel()

	results := make(chan error, len(s.actors))
	for _, actor := range s.actors {
		actor := actor
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
			"restart_window": s.config.restartWindow,
			"restart_delay":  s.config.restartDelay,
			"max_restarts":   s.config.maxRestarts,
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
