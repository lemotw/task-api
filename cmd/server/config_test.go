package main

import (
	"testing"
	"time"

	"github.com/lemotw/task-api/internal/logging"
)

func envOf(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestParseConfig_Defaults(t *testing.T) {
	got, err := parseConfig(envOf(nil))
	if err != nil {
		t.Fatalf("parseConfig = %v", err)
	}
	want := config{
		Port:          "8080",
		LogLevel:      logging.LevelInfo,
		LogOutput:     "stdout",
		DataFile:      "",
		FlushInterval: 5 * time.Second,
	}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestParseConfig_ReadsEveryVariable(t *testing.T) {
	got, err := parseConfig(envOf(map[string]string{
		"PORT":                "9000",
		"LOG_LEVEL":           "debug",
		"LOG_OUTPUT":          "stdout",
		"DATA_FILE":           "/data/tasks.json",
		"DATA_FLUSH_INTERVAL": "250ms",
	}))
	if err != nil {
		t.Fatalf("parseConfig = %v", err)
	}
	want := config{
		Port:          "9000",
		LogLevel:      logging.LevelDebug,
		LogOutput:     "stdout",
		DataFile:      "/data/tasks.json",
		FlushInterval: 250 * time.Millisecond,
	}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestParseConfig_ZeroIntervalDisablesPeriodicFlush(t *testing.T) {
	got, err := parseConfig(envOf(map[string]string{"DATA_FLUSH_INTERVAL": "0"}))
	if err != nil {
		t.Fatalf("parseConfig = %v", err)
	}
	if got.FlushInterval != 0 {
		t.Errorf("FlushInterval = %v", got.FlushInterval)
	}
}

func TestParseConfig_RejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"unknown log level":     {"LOG_LEVEL": "TRACE"},
		"unsupported output":    {"LOG_OUTPUT": "udp://127.0.0.1:5140"},
		"file output":           {"LOG_OUTPUT": "file:///tmp/app.log"},
		"interval not duration": {"DATA_FLUSH_INTERVAL": "abc"},
		"interval without unit": {"DATA_FLUSH_INTERVAL": "5"},
		"negative interval":     {"DATA_FLUSH_INTERVAL": "-1s"},
	}
	for label, env := range cases {
		t.Run(label, func(t *testing.T) {
			if _, err := parseConfig(envOf(env)); err == nil {
				t.Error("parseConfig returned no error")
			}
		})
	}
}

func TestDefaultShutdownTimeout_MatchesSpec(t *testing.T) {
	if defaultShutdownTimeout != 5*time.Second {
		t.Errorf("defaultShutdownTimeout = %v, want 5s", defaultShutdownTimeout)
	}
}
