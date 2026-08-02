// Package process adapts standard-library os/exec commands to sup actors.
package process

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"

	"github.com/webermarci/sup"
)

var (
	// ErrActorRunning is returned when Run is called while the actor is already
	// running a process.
	ErrActorRunning = errors.New("process: actor is already running")

	// ErrNilCommand is returned when a CommandFunc returns nil.
	ErrNilCommand = errors.New("process: command function returned nil")
)

// CommandFunc creates a fresh command for one execution attempt. The supplied
// context should normally be passed to os/exec.CommandContext so cancellation
// stops the process. A command cannot be reused across calls.
type CommandFunc func(context.Context) *exec.Cmd

// Actor creates and runs an external command for each execution attempt.
// Process configuration and cancellation behavior belong to CommandFunc; the
// actor only adapts the command lifecycle to sup.Actor.
type Actor struct {
	id      string
	command CommandFunc
	running atomic.Bool
}

var _ sup.Actor = (*Actor)(nil)
var _ sup.Inspectable = (*Actor)(nil)

// NewActor creates a process actor with the given id and command function.
func NewActor(id string, command CommandFunc) *Actor {
	if id == "" {
		panic("process: actor id cannot be empty")
	}

	if command == nil {
		panic("process: command function cannot be nil")
	}

	return &Actor{id: id, command: command}
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Inspect returns the process actor spec.
func (a *Actor) Inspect() sup.Spec {
	return sup.Spec{Kind: "process"}
}

// Run creates and runs one command. Context cancellation is a clean actor
// shutdown and returns nil; other command errors are returned to the
// supervisor.
func (a *Actor) Run(ctx context.Context) error {
	if ctx == nil {
		panic("process: context cannot be nil")
	}

	if !a.running.CompareAndSwap(false, true) {
		return ErrActorRunning
	}
	defer a.running.Store(false)

	cmd := a.command(ctx)
	if cmd == nil {
		return ErrNilCommand
	}

	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}
