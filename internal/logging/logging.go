// Package logging encodes log events and writes them to a Sink.
// See docs/spec/core/04-log.md.
package logging

import "time"

// Level is the event severity (LOG-7).
type Level int

// Levels from lowest to highest (LOG-9).
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the uppercase name written to the log (LOG-7).
func (l Level) String() string {
	panic("not implemented")
}

// ParseLevel parses LOG_LEVEL, ignoring case (LOG-8).
func ParseLevel(s string) (Level, error) {
	panic("not implemented")
}

// Field is an event-specific field, written at the top level of the JSON
// object (LOG-6).
type Field struct {
	Key   string
	Value any
}

// AccessEvent holds the fields of an http.request event (LOG-11).
type AccessEvent struct {
	Method     string
	Route      string
	Path       string
	Status     int
	DurationMS int64
	Bytes      int64
}

// Logger is what callers depend on. Methods return no error because a logging
// failure MUST NOT affect a request (LOG-18). Implementations MUST be safe for
// concurrent use (LOG-4).
type Logger interface {
	// Debug, Info, Warn and Error emit log_type=debug events, filtered by
	// LOG_LEVEL (LOG-9).
	Debug(event string, fields ...Field)
	Info(event string, fields ...Field)
	Warn(event string, fields ...Field)
	Error(event string, fields ...Field)
	// Access emits a log_type=access event, never filtered (LOG-10).
	Access(e AccessEvent)
}

// Sink is an output destination, kept apart from encoding (LOG-3). Write
// receives one encoded event.
type Sink interface {
	Write(line []byte) error
	Close() error
}

// Config configures JSONLogger.
type Config struct {
	Level    Level
	Instance string
	Sink     Sink
	Now      func() time.Time
}

var _ Logger = (*JSONLogger)(nil)

// JSONLogger writes each event as one JSON object per line (LOG-1).
type JSONLogger struct {
	cfg Config
}

// New returns a JSONLogger.
func New(cfg Config) *JSONLogger {
	panic("not implemented")
}

// Debug implements Logger.
func (l *JSONLogger) Debug(event string, fields ...Field) {
	panic("not implemented")
}

// Info implements Logger.
func (l *JSONLogger) Info(event string, fields ...Field) {
	panic("not implemented")
}

// Warn implements Logger.
func (l *JSONLogger) Warn(event string, fields ...Field) {
	panic("not implemented")
}

// Error implements Logger.
func (l *JSONLogger) Error(event string, fields ...Field) {
	panic("not implemented")
}

// Access implements Logger.
func (l *JSONLogger) Access(e AccessEvent) {
	panic("not implemented")
}

// Close closes the Sink. It is called last during shutdown (LOG-19).
func (l *JSONLogger) Close() error {
	panic("not implemented")
}
