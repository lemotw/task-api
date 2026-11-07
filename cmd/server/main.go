// Command server runs the task API.
package main

import (
	"context"
	"io"
	"os"

	"github.com/lemo/task-api/internal/httpapi"
	"github.com/lemo/task-api/internal/memory"
	"github.com/lemo/task-api/internal/snapshot"
	"github.com/lemo/task-api/internal/task"
)

// Compile-time checks that the layers fit together.
var (
	_ httpapi.TaskService = (*task.Service)(nil)
	_ snapshot.Source     = (*memory.Repository)(nil)
)

func main() {
	os.Exit(run(context.Background(), os.Getenv, os.Stdout))
}

// run wires the layers and serves until ctx is cancelled or a shutdown signal
// arrives. It returns the process exit code (ARC-19, PER-22).
func run(ctx context.Context, getenv func(string) string, stdout io.Writer) int {
	panic("not implemented")
}
