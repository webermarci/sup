package process

import (
	"context"
	"os/exec"

	"github.com/webermarci/sup"
)

// Actor creates and runs an external command for each execution attempt.
// Process configuration and cancellation behavior belong to the command
// factory; the actor only adapts the command lifecycle to sup.Actor.
type Actor struct {
	id      string
	command func(context.Context) *exec.Cmd
}

var _ sup.Actor = (*Actor)(nil)

// NewActor creates a process actor with the given id and command factory. The
// factory must return a fresh command for every execution attempt. It should
// normally pass the supplied context to exec.CommandContext so cancellation
// stops the process.
func NewActor(id string, command func(context.Context) *exec.Cmd) *Actor {
	if id == "" {
		panic("process: actor id cannot be empty")
	}
	return &Actor{id: id, command: command}
}

// ID returns the actor id.
func (a *Actor) ID() string {
	return a.id
}

// Run creates and runs one command. Context cancellation is a clean actor
// shutdown and returns nil; other command errors are returned to the
// supervisor.
func (a *Actor) Run(ctx context.Context) error {
	cmd := a.command(ctx)
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}
