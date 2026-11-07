package main

import (
	"fmt"
	"time"

	"github.com/lemotw/task-api/internal/logging"
)

const (
	defaultPort          = "8080"
	defaultFlushInterval = 5 * time.Second
	logOutputStdout      = "stdout"
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
	cfg := config{
		Port:          valueOr(getenv("PORT"), defaultPort),
		LogLevel:      logging.LevelInfo,
		LogOutput:     valueOr(getenv("LOG_OUTPUT"), logOutputStdout),
		DataFile:      getenv("DATA_FILE"),
		FlushInterval: defaultFlushInterval,
	}

	if v := getenv("LOG_LEVEL"); v != "" {
		level, err := logging.ParseLevel(v)
		if err != nil {
			return config{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
		cfg.LogLevel = level
	}
	if cfg.LogOutput != logOutputStdout {
		return config{}, fmt.Errorf("LOG_OUTPUT: unsupported value %q, only %q is supported", cfg.LogOutput, logOutputStdout)
	}
	if v := getenv("DATA_FLUSH_INTERVAL"); v != "" {
		interval, err := time.ParseDuration(v)
		if err != nil {
			return config{}, fmt.Errorf("DATA_FLUSH_INTERVAL: %w", err)
		}
		if interval < 0 {
			return config{}, fmt.Errorf("DATA_FLUSH_INTERVAL: %s is negative", v)
		}
		cfg.FlushInterval = interval
	}
	return cfg, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
