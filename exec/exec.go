package exec

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/webermarci/sup"
)

// Actor wraps an external OS command and runs it under supervision.
// If the command exits, the actor returns the error, allowing the supervisor to restart it.
type Actor struct {
	*sup.BaseActor
	path      string
	args      []string
	dir       string
	env       []string
	stdout    io.Writer
	stderr    io.Writer
	waitDelay time.Duration
}

// ActorOption defines a function type for configuring an exec Actor.
type ActorOption func(*Actor)

// WithDir sets the working directory for the command.
func WithDir(dir string) ActorOption {
	return func(a *Actor) {
		a.dir = dir
	}
}

// WithEnv sets the environment variables for the command.
func WithEnv(env []string) ActorOption {
	return func(a *Actor) {
		a.env = env
	}
}

// WithStdout redirects the command's standard output.
func WithStdout(w io.Writer) ActorOption {
	return func(a *Actor) {
		a.stdout = w
	}
}

// WithStderr redirects the command's standard error.
func WithStderr(w io.Writer) ActorOption {
	return func(a *Actor) {
		a.stderr = w
	}
}

// WithWaitDelay sets the time to wait for the process to exit after a cancellation signal is sent.
// Defaults to 0 (immediate SIGKILL).
func WithWaitDelay(d time.Duration) ActorOption {
	return func(a *Actor) {
		a.waitDelay = d
	}
}

// NewActor creates a new exec Actor with the given name and command path.
func NewActor(name string, path string, args []string, opts ...ActorOption) *Actor {
	a := &Actor{
		BaseActor: sup.NewBaseActor(name),
		path:      path,
		args:      args,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Run executes the command and waits for it to finish.
// The process is automatically killed if the context is canceled.
func (a *Actor) Run(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.path, a.args...)
	cmd.Dir = a.dir
	cmd.Env = a.env
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr

	// Graceful shutdown: send Interrupt first, wait a bit, then SIGKILL.
	if a.waitDelay > 0 {
		cmd.Cancel = func() error {
			return cmd.Process.Signal(os.Interrupt)
		}
		cmd.WaitDelay = a.waitDelay
	}

	err := cmd.Run()

	if ctx.Err() != nil {
		return nil
	}

	return err
}
