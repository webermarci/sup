package process_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/process"
)

const (
	helperEnabled = "SUP_EXEC_HELPER"
	helperMode    = "SUP_EXEC_HELPER_MODE"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnabled) != "1" {
		return
	}

	separator := slices.Index(os.Args, "--")
	var args []string
	if separator >= 0 {
		args = os.Args[separator+1:]
	}

	switch os.Getenv(helperMode) {
	case "echo":
		fmt.Fprintln(os.Stdout, strings.Join(args, " "))
	case "wait":
		fmt.Fprintln(os.Stdout, "ready")
		for {
			time.Sleep(time.Hour)
		}
	default:
		os.Exit(2)
	}

	os.Exit(0)
}

func helperCommand(ctx context.Context, mode string, args ...string) *exec.Cmd {
	commandArgs := []string{"-test.run=^TestHelperProcess$", "--"}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), helperEnabled+"=1", helperMode+"="+mode)
	return cmd
}

func newHelperActor(id string, mode string, stdout io.Writer, args ...string) *process.Actor {
	return process.NewActor(id, func(ctx context.Context) *exec.Cmd {
		cmd := helperCommand(ctx, mode, args...)
		cmd.Stdout = stdout
		return cmd
	})
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

type readyWriter struct {
	ready chan struct{}
	once  sync.Once
}

func (w *readyWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("ready")) {
		w.once.Do(func() { close(w.ready) })
	}
	return len(p), nil
}

func TestActorRun(t *testing.T) {
	var stdout bytes.Buffer
	actor := newHelperActor("echo", "echo", &stdout, "hello", "world")

	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := stdout.String(), "hello world\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestActorCreatesCommandForEveryRun(t *testing.T) {
	var commands atomic.Int32
	actor := process.NewActor("repeat", func(ctx context.Context) *exec.Cmd {
		commands.Add(1)
		return helperCommand(ctx, "echo")
	})

	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("unexpected first-run error: %v", err)
	}
	if err := actor.Run(t.Context()); err != nil {
		t.Fatalf("unexpected second-run error: %v", err)
	}
	if commands.Load() != 2 {
		t.Fatalf("expected two fresh commands, got %d", commands.Load())
	}
}

func TestActorPassesRunContextToCommand(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "value")
	var received string
	actor := process.NewActor("context", func(commandCtx context.Context) *exec.Cmd {
		received, _ = commandCtx.Value(contextKey{}).(string)
		return helperCommand(commandCtx, "echo")
	})

	if err := actor.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "value" {
		t.Fatalf("command did not receive run context: got %q", received)
	}
}

func TestActorReturnsProcessFailure(t *testing.T) {
	actor := newHelperActor("failure", "fail", nil)
	err := actor.Run(t.Context())
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("expected process exit error, got %v", err)
	}
}

func TestActorSupervision(t *testing.T) {
	var starts atomic.Int32
	actor := newHelperActor("process", "echo", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	supervisor := sup.NewSupervisor("sup",
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(10*time.Millisecond),
		sup.WithEventSink(sup.EventSinkFunc(func(event sup.Event) {
			if event.Type == sup.EventActorStarted && starts.Add(1) == 2 {
				cancel()
			}
		})),
		sup.WithActors(actor),
	)

	if err := supervisor.Run(ctx); err != nil {
		t.Fatalf("unexpected supervisor error: %v", err)
	}
	if starts.Load() != 2 {
		t.Errorf("expected two starts, got %d", starts.Load())
	}
}

func TestActorCancellation(t *testing.T) {
	ready := &readyWriter{ready: make(chan struct{})}
	actor := newHelperActor("wait", "wait", ready)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	select {
	case <-ready.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not start")
	}
	start := time.Now()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected nil error on cancellation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("expected quick cancellation, took %v", elapsed)
	}
}

func TestActorRejectsConcurrentRun(t *testing.T) {
	ready := &readyWriter{ready: make(chan struct{})}
	actor := newHelperActor("wait", "wait", ready)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- actor.Run(ctx) }()

	select {
	case <-ready.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not start")
	}

	if err := actor.Run(t.Context()); !errors.Is(err, process.ErrActorRunning) {
		t.Fatalf("expected ErrActorRunning, got %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
}

func TestActorInspect(t *testing.T) {
	actor := newHelperActor("process", "echo", nil)
	if actor.ID() != "process" {
		t.Fatalf("unexpected actor id: %q", actor.ID())
	}
	if spec := actor.Inspect(); spec.Kind != "process" {
		t.Fatalf("unexpected actor spec: %+v", spec)
	}
}

func TestNewActorValidation(t *testing.T) {
	command := func(ctx context.Context) *exec.Cmd { return helperCommand(ctx, "echo") }
	requirePanic(t, func() { process.NewActor("", command) })
	requirePanic(t, func() { process.NewActor("actor", nil) })
}

func TestActorRejectsNilCommand(t *testing.T) {
	actor := process.NewActor("nil", func(context.Context) *exec.Cmd { return nil })
	if err := actor.Run(t.Context()); !errors.Is(err, process.ErrNilCommand) {
		t.Fatalf("expected ErrNilCommand, got %v", err)
	}
}
