package resource_test

import (
	"bytes"
	"context"
	"fmt"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/resource"
)

func ExampleActor_Call() {
	ctx, cancel := context.WithCancel(context.Background())
	buffer := resource.NewActor(
		"buffer",
		func(context.Context) (*bytes.Buffer, error) {
			return new(bytes.Buffer), nil
		},
		func(context.Context, *bytes.Buffer) error {
			return nil
		},
	)

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(buffer)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	length, err := buffer.Call(ctx,
		func(_ context.Context, buffer *bytes.Buffer) (int, error) {
			return buffer.WriteString("hello")
		})
	if err != nil {
		panic(err)
	}
	fmt.Println("length:", length)

	if err := buffer.Cast(ctx,
		func(_ context.Context, buffer *bytes.Buffer) error {
			return buffer.WriteByte('!')
		}); err != nil {
		panic(err)
	}

	value, err := buffer.Call(ctx,
		func(_ context.Context, buffer *bytes.Buffer) (string, error) {
			return buffer.String(), nil
		})
	if err != nil {
		panic(err)
	}
	fmt.Println("value:", value)

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}

	// Output:
	// length: 5
	// value: hello!
}
