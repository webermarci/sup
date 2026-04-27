package exec_test

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/exec"
)

func TestActor_Run(t *testing.T) {
	var stdout bytes.Buffer
	actor := exec.NewActor("echo", "echo", []string{"hello", "world"}, exec.WithStdout(&stdout))

	err := actor.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stdout.String()
	want := "hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestActor_Supervision(t *testing.T) {
	var runs atomic.Int32
	// Use a script that exits immediately to trigger restarts
	actor := exec.NewActor("restart-test", "go", []string{"version"})

	supervisor := sup.NewSupervisor("sup",
		sup.WithActor(sup.ActorFunc("wrapped", func(ctx context.Context) error {
			runs.Add(1)
			return actor.Run(ctx)
		})),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = supervisor.Run(ctx)

	if runs.Load() <= 1 {
		t.Errorf("expected multiple runs, got %d", runs.Load())
	}
}

func TestActor_Cancellation(t *testing.T) {
	// A command that sleeps for a long time
	actor := exec.NewActor("sleep", "sleep", []string{"10"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := actor.Run(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error on cancellation, got %v", err)
	}

	if duration > 1*time.Second {
		t.Errorf("expected quick cancellation, took %v", duration)
	}
}
