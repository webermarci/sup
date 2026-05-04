# sup/hub

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/sup/hub.svg)](https://pkg.go.dev/github.com/webermarci/sup/hub)
[![Test](https://github.com/webermarci/sup/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/sup/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

`hub` is a high-performance, generic load balancer and distribution utility for Go. Designed as a companion for the **sup** actor library, it allows you to group multiple function signatures (routees) and call them using various strategies like Round-Robin, Random selection, or Fixed affinity.

It is **stateless**, **thread-safe**, and designed to be virtually overhead-free in your hot path.

## Features

- **Generic-first**: Works with any function signature or type using Go Generics.
- **Multiple Strategies**: Built-in Round-Robin and Random selection.
- **Resilience**: Built-in `Retry` logic with automatic failover and error propagation.
- **Bulk Operations**: `Broadcast` for sequential updates and `FanOut` for parallel execution.

## Installation

```bash
go get github.com/webermarci/sup/hub
```

## Quick Start

```go
package main

import (
	"github.com/webermarci/sup"
	"github.com/webermarci/sup/hub"
)

type Worker struct{
}

func (w *Worker) Process(data string) error {
  if data == "fail" {
    return fmt.Errorf("processing failed")
  }
  return nil
}

func main() {
	w1 := Worker{}
	w2 := Worker{}

	// Create a Round-Robin Hub for the "Process" capability
	workers := hub.NewRoundRobin(w1.Process, w2.Process)

	if err := workers.Next()("task-data"); err != nil {
		fmt.Println("Error processing task:", err)
	}
	
	workers.Broadcast("update-data") // Broadcast to all workers
	workser.FanOut("parallel-task-data") // Fan-out to all workers in parallel
	workers.FanOutWait("parallel-task-data") // Fan-out and wait for all to complete

	if err := workers.Retry(3, func(fn func(data string) error) error {
		return fn("task-data") // Just call the function, but you could add backoff, logging, etc. here
	}); err != nil {
    fmt.Println("All retries failed:", err)
  }
}
```
