// Package process adapts standard-library os/exec commands to sup actors.
//
// # Getting started
//
// Create a process Actor with a command factory, then place it under a sup
// Supervisor:
//
//	worker := process.NewActor("worker", func(ctx context.Context) *exec.Cmd {
//		return exec.CommandContext(ctx, "./worker", "--serve")
//	})
//
//	supervisor := sup.NewSupervisor("root",
//		sup.WithPolicy(sup.Transient),
//		sup.WithActors(worker),
//	)
//
// The command factory is called once for every execution attempt and must
// return a fresh *exec.Cmd. Configure environment, working directory, output,
// and cancellation on that command using the standard library.
//
// Use exec.CommandContext with the supplied context when cancellation should
// stop the process. Context cancellation is a clean actor shutdown and makes
// Run return nil. Other command errors are returned to the supervisor and are
// handled by its restart policy.
//
// A command factory that returns nil produces ErrNilCommand. Concurrent calls
// to Run produce ErrActorRunning. See the package README and the package
// reference at https://pkg.go.dev/github.com/webermarci/sup/process for the
// complete API.
package process
