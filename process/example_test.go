package process_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/process"
)

func ExampleActor() {
	command := process.NewActor("hello", func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "sh", "-c", "printf 'hello from process actor\\n'")
		cmd.Stdout = os.Stdout
		return cmd
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(command)
	if err := supervisor.Run(context.Background()); err != nil {
		panic(err)
	}
	fmt.Println("process finished")

	// Output:
	// hello from process actor
	// process finished
}
