package main

import (
	"time"

	"github.com/lemo/task-api/internal/logging"
)

// config holds the settings read from the environment.
type config struct {
	Port          string        // PORT (ARC-10)
	LogLevel      logging.Level // LOG_LEVEL (LOG-8)
	LogOutput     string        // LOG_OUTPUT (LOG-2)
	DataFile      string        // DATA_FILE (PER-1)
	FlushInterval time.Duration // DATA_FLUSH_INTERVAL (PER-2)
}

// parseConfig reads settings through getenv and rejects invalid values
// (LOG-2, LOG-8, PER-2).
func parseConfig(getenv func(string) string) (config, error) {
	panic("not implemented")
}
