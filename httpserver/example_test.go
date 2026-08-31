package httpserver_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/webermarci/sup"
	"github.com/webermarci/sup/httpserver"
)

func ExampleActor() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	addresses := make(chan string, 1)

	server := httpserver.NewActor("api", func(context.Context) (*http.Server, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})
		return &http.Server{Handler: mux}, nil
	}).SetServeFunc(func(_ context.Context, server *http.Server) error {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		addresses <- listener.Addr().String()
		return server.Serve(listener)
	})

	supervisor := sup.NewSupervisor("root", sup.Temporary).
		AddActors(server)
	done := make(chan error, 1)
	go func() { done <- supervisor.Run(ctx) }()

	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	response, err := client.Get("http://" + <-addresses + "/health")
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	_ = response.Body.Close()
	fmt.Println(response.StatusCode, string(body))

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}

	// Output:
	// 200 ok
}
