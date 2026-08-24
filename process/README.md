# sup/process

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/process.svg)](https://pkg.go.dev/github.com/webermarci/sup/process)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`sup/process` is a small adapter between `os/exec.Cmd` and `sup.Actor`. The caller
creates and configures an ordinary standard-library command; the actor runs one
fresh command for each execution attempt.

## Usage

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/process"
)

func main() {
	worker := process.NewActor("worker", func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "./worker", "--serve")
		cmd.Dir = "/srv/worker"
		cmd.Env = append(os.Environ(), "LOG_LEVEL=info")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd
	})

	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(worker)

	if err := supervisor.Run(context.Background()); err != nil {
		fmt.Println("supervisor failed:", err)
	}
}
```

## Contract

- The command function is called once per execution attempt and must return a
  fresh `*os/exec.Cmd`; commands cannot be reused.
- Use `os/exec.CommandContext` with the supplied context when cancellation
  should stop the process.
- All command configuration remains standard Go: stdin, output, environment,
  working directory, platform attributes, cancellation, and `WaitDelay` are
  configured directly on `os/exec.Cmd`.
- Context cancellation is a clean actor shutdown and makes `Run` return `nil`.
- Other command errors are returned to the supervisor.

Process restarts, restart delays, restart limits, logging, and application state
belong to the supervisor or application rather than this package.
