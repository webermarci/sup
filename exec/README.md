# sup/exec

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/exec.svg)](https://pkg.go.dev/github.com/webermarci/sup/exec)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`exec` allows external OS processes to be managed as `sup` actors. It is designed for supervising sidecar binaries, legacy tools, or long-running scripts as part of your application's actor tree.

## Installation

```bash
go get github.com/webermarci/sup/exec
```

## Concepts

The `exec.Actor` wraps the standard library's `os/exec.Cmd`. It integrates with `sup.Supervisor` to provide:

- **Automatic Restarts** — If the process crashes or exits unexpectedly, the supervisor resurrects it.
- **Graceful Shutdown** — Processes can be configured to receive an interrupt signal (SIGINT) before being forcibly killed.
- **Output Redirection** — Capture `stdout` and `stderr` to any `io.Writer`.

## Usage

Create an actor by specifying the executable path and its arguments.

```go
// Run a simple echo command
actor := exec.NewActor("echo", "echo", []string{"hello", "world"})

// Or run a long-running sidecar process
dbSidecar := exec.NewActor("db-proxy", "./bin/proxy", []string{"--port", "5432"},
	exec.WithStdout(os.Stdout),
	exec.WithWaitDelay(5*time.Second),
)

// Manage it with a supervisor
supervisor := sup.NewSupervisor("root",
	sup.WithActor(dbSidecar),
	sup.WithPolicy(sup.Permanent),
)

go supervisor.Run(ctx)
```

### Options

| Option | Default | Description |
|---|---|---|
| `WithDir(path)` | current | Set the working directory for the process |
| `WithEnv([]string)` | inherited | Set environment variables (e.g., `["KEY=VALUE"]`) |
| `WithStdout(w)` | nil | Redirect standard output to an `io.Writer` |
| `WithStderr(w)` | nil | Redirect standard error to an `io.Writer` |
| `WithWaitDelay(d)` | 0 | Graceful shutdown: send SIGINT and wait `d` before SIGKILL |

### Behaviour

- **Clean Exit:** If the context is canceled, the actor returns `nil`. This ensures that a supervisor shutting down does not interpret the process termination as a failure.
- **Graceful Shutdown:** If `WithWaitDelay` is set, the actor sends `os.Interrupt` to the process when the context is canceled. It waits for the specified duration before the standard library's `CommandContext` triggers a `SIGKILL`.
- **Environment:** By default, the process inherits the environment of the parent Go process unless overridden by `WithEnv`.

## Full Example

```go
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Define a sidecar actor (e.g., a local metrics exporter)
	exporter := exec.NewActor("exporter", "./metrics-exporter", []string{"--interval", "1s"},
		exec.WithStdout(os.Stdout),
		exec.WithWaitDelay(2*time.Second),
	)

	// 2. Wrap it in a supervisor
	supervisor := sup.NewSupervisor("sidecars",
		sup.WithActor(exporter),
		sup.WithPolicy(sup.Permanent),
		sup.WithRestartDelay(1*time.Second),
		sup.WithOnError(func(actor sup.Actor, err error) {
			log.Printf("Process %s exited with error: %v", actor.Name(), err)
		}),
	)

	// 3. Run blocks until context is canceled or a terminal error occurs
	if err := supervisor.Run(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Fatalf("Supervisor failed: %v", err)
	}
}
```
