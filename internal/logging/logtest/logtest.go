// Package logtest provides a logging.Logger that records events for tests.
package logtest

import (
	"sync"

	"github.com/lemotw/task-api/internal/logging"
)

var _ logging.Logger = (*Recorder)(nil)

// Entry is a recorded debug event.
type Entry struct {
	Level  logging.Level
	Event  string
	Fields map[string]any
}

// Recorder records events. It is safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	entries []Entry
	access  []logging.AccessEvent
}

func (r *Recorder) record(level logging.Level, event string, fields []logging.Field) {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, Entry{Level: level, Event: event, Fields: m})
}

// Debug implements logging.Logger.
func (r *Recorder) Debug(event string, fields ...logging.Field) {
	r.record(logging.LevelDebug, event, fields)
}

// Info implements logging.Logger.
func (r *Recorder) Info(event string, fields ...logging.Field) {
	r.record(logging.LevelInfo, event, fields)
}

// Warn implements logging.Logger.
func (r *Recorder) Warn(event string, fields ...logging.Field) {
	r.record(logging.LevelWarn, event, fields)
}

// Error implements logging.Logger.
func (r *Recorder) Error(event string, fields ...logging.Field) {
	r.record(logging.LevelError, event, fields)
}

// Access implements logging.Logger.
func (r *Recorder) Access(e logging.AccessEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.access = append(r.access, e)
}

// Find returns the recorded debug events with the given name.
func (r *Recorder) Find(event string) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Entry
	for _, e := range r.entries {
		if e.Event == event {
			out = append(out, e)
		}
	}
	return out
}

// AccessEvents returns the recorded access events.
func (r *Recorder) AccessEvents() []logging.AccessEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]logging.AccessEvent(nil), r.access...)
}
