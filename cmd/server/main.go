// Command server runs the task API.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lemo/task-api/internal/httpapi"
	"github.com/lemo/task-api/internal/logging"
	"github.com/lemo/task-api/internal/memory"
	"github.com/lemo/task-api/internal/snapshot"
	"github.com/lemo/task-api/internal/task"
	"github.com/lemo/task-api/internal/webui"
)

// defaultShutdownTimeout bounds the wait for in-flight requests (ARC-18).
const defaultShutdownTimeout = 5 * time.Second

const readHeaderTimeout = 5 * time.Second

const (
	exitOK      = 0
	exitFailure = 1
)

// Compile-time checks that the layers fit together.
var (
	_ httpapi.TaskService = (*task.Service)(nil)
	_ snapshot.Source     = (*memory.Repository)(nil)
)

// runOptions are the process inputs of run, injectable for tests.
type runOptions struct {
	Getenv          func(string) string
	Stdout          io.Writer
	ShutdownTimeout time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, runOptions{
		Getenv:          os.Getenv,
		Stdout:          os.Stdout,
		ShutdownTimeout: defaultShutdownTimeout,
	})
	stop()
	os.Exit(code)
}

// run wires the layers and serves until ctx is cancelled or a shutdown signal
// arrives. It returns the process exit code (ARC-19, ARC-20, PER-22).
func run(ctx context.Context, opts runOptions) int {
	cfg, cfgErr := parseConfig(opts.Getenv)

	// The logger starts first and stops last (LOG-19). An invalid configuration
	// is still reported, at the default level.
	level := logging.LevelInfo
	if cfgErr == nil {
		level = cfg.LogLevel
	}
	logger := logging.New(logging.Config{
		Level:    level,
		Instance: newInstanceID(),
		Sink:     logging.NewWriterSink(opts.Stdout),
	})
	defer logger.Close()

	if cfgErr != nil {
		return startFailed(logger, cfgErr)
	}

	repo := memory.New()
	var syncer *snapshot.Syncer
	if cfg.DataFile != "" {
		syncer = snapshot.NewSyncer(repo, snapshot.NewFile(cfg.DataFile), cfg.FlushInterval, logger)
		// The snapshot is loaded before any request is accepted (PER-5).
		if err := syncer.Load(); err != nil {
			return startFailed(logger, err)
		}
	}

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return startFailed(logger, err)
	}
	server := &http.Server{
		Handler: httpapi.NewHandler(httpapi.Deps{
			Tasks: task.NewService(repo),
			Log:   logger,
			Pages: webui.Pages(),
		}),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	logger.Info("server.started", logging.Field{Key: "addr", Value: listener.Addr().String()})

	syncCtx, stopSync := context.WithCancel(context.Background())
	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		if syncer != nil {
			syncer.Run(syncCtx)
		}
	}()

	code := exitOK
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		logger.Error("server.serve_failed", logging.Field{Key: "error", Value: err.Error()})
		code = exitFailure
	}

	// Shutdown order (PER-20): stop accepting and drain requests, stop the
	// periodic flush, write the last changes, then stop logging.
	if !drain(server, opts.ShutdownTimeout, logger) {
		code = exitFailure // ARC-20
	}
	stopSync()
	<-syncDone
	if syncer != nil {
		if err := syncer.Flush(); err != nil { // Flush logs the failure
			code = exitFailure // PER-22
		}
	}
	logger.Info("server.stopped")
	return code
}

// drain stops the server and waits for in-flight requests (ARC-18). It reports
// false if the wait timed out; remaining connections are then closed.
func drain(server *http.Server, timeout time.Duration, logger logging.Logger) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := server.Shutdown(ctx)
	if err == nil {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Warn("server.shutdown_timeout")
	} else {
		logger.Error("server.shutdown_failed", logging.Field{Key: "error", Value: err.Error()})
	}
	_ = server.Close()
	return false
}

func startFailed(logger logging.Logger, err error) int {
	logger.Error("server.start_failed", logging.Field{Key: "error", Value: err.Error()})
	return exitFailure
}

// newInstanceID identifies this process in log events (LOG-5).
func newInstanceID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
