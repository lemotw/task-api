// Package logging encodes log events and writes them to a Sink.
// See docs/spec/core/04-log.md.
package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	schemaVersion = 1
	serviceName   = "task-api"
	timeLayout    = "2006-01-02T15:04:05.000Z"

	// maxEventBytes limits one encoded event, including its newline (LOG-17).
	maxEventBytes = 1400

	logTypeAccess = "access"
	logTypeDebug  = "debug"
	accessEvent   = "http.request"
)

// reservedKeys are the field names custom fields cannot use (LOG-6, LOG-24).
var reservedKeys = map[string]bool{
	"v": true, "log_type": true, "time": true, "level": true, "service": true,
	"instance": true, "seq": true, "event": true, "truncated": true,
}

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
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return fmt.Sprintf("Level(%d)", int(l))
	}
}

// ParseLevel parses LOG_LEVEL, ignoring case (LOG-8).
func ParseLevel(s string) (Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", s)
	}
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
// receives one encoded event ending with exactly one newline; the newline
// counts toward the event size limit (LOG-17).
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

	mu  sync.Mutex // serializes seq and Sink so output order matches seq
	seq uint64
}

// New returns a JSONLogger.
func New(cfg Config) *JSONLogger {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &JSONLogger{cfg: cfg}
}

// Debug implements Logger.
func (l *JSONLogger) Debug(event string, fields ...Field) {
	l.debugEvent(LevelDebug, event, fields)
}

// Info implements Logger.
func (l *JSONLogger) Info(event string, fields ...Field) {
	l.debugEvent(LevelInfo, event, fields)
}

// Warn implements Logger.
func (l *JSONLogger) Warn(event string, fields ...Field) {
	l.debugEvent(LevelWarn, event, fields)
}

// Error implements Logger.
func (l *JSONLogger) Error(event string, fields ...Field) {
	l.debugEvent(LevelError, event, fields)
}

// Access implements Logger.
func (l *JSONLogger) Access(e AccessEvent) {
	level := LevelInfo
	if e.Status >= 500 {
		level = LevelError
	}
	l.emit(logTypeAccess, level, accessEvent, []Field{
		{"method", e.Method},
		{"route", e.Route},
		{"path", e.Path},
		{"status", e.Status},
		{"duration_ms", e.DurationMS},
		{"bytes", e.Bytes},
	})
}

// Close closes the Sink. It is called last during shutdown (LOG-19).
func (l *JSONLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cfg.Sink.Close()
}

func (l *JSONLogger) debugEvent(level Level, event string, fields []Field) {
	if level < l.cfg.Level {
		return
	}
	l.emit(logTypeDebug, level, event, fields)
}

func (l *JSONLogger) emit(logType string, level Level, event string, fields []Field) {
	custom := normalizeFields(fields)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	shared := []Field{
		{"v", schemaVersion},
		{"log_type", logType},
		{"time", l.cfg.Now().UTC().Format(timeLayout)},
		{"level", level.String()},
		{"service", serviceName},
		{"instance", l.cfg.Instance},
		{"seq", l.seq},
		{"event", event},
	}
	// A failed write is dropped and leaves a seq gap for readers (LOG-18).
	_ = l.cfg.Sink.Write(encodeEvent(shared, custom))
}

// normalizeFields drops reserved names (LOG-24) and turns values into a form
// that encodes as readable JSON.
func normalizeFields(fields []Field) []Field {
	out := make([]Field, 0, len(fields))
	for _, f := range fields {
		if reservedKeys[f.Key] {
			continue
		}
		switch v := f.Value.(type) {
		case error:
			f.Value = v.Error()
		case string:
			f.Value = strings.ToValidUTF8(v, "�")
		}
		out = append(out, f)
	}
	return out
}

// encodeEvent encodes the event, truncating custom fields past the size limit.
func encodeEvent(shared, custom []Field) []byte {
	line := encodeLine(shared, custom)
	if len(line) <= maxEventBytes {
		return line
	}
	shared = append(shared, Field{"truncated", true})

	// The budget is what remains with every string value empty.
	shrunk := make([]Field, len(custom))
	copy(shrunk, custom)
	var strs []int
	for i, f := range shrunk {
		if _, ok := f.Value.(string); ok {
			shrunk[i].Value = ""
			strs = append(strs, i)
		}
	}
	line = encodeLine(shared, shrunk)
	for len(line) > maxEventBytes && len(shrunk) > 0 { // oversized non-strings
		shrunk = shrunk[:len(shrunk)-1]
		line = encodeLine(shared, shrunk)
	}
	if len(shrunk) < len(custom) {
		return line
	}

	// Short values go first so they stay whole; each gets a fair share.
	budget := maxEventBytes - len(line)
	sort.SliceStable(strs, func(a, b int) bool {
		return len(custom[strs[a]].Value.(string)) < len(custom[strs[b]].Value.(string))
	})
	for n, i := range strs {
		share := budget / (len(strs) - n)
		kept := prefixWithin(custom[i].Value.(string), share)
		shrunk[i].Value = kept
		budget -= encodedLen(kept)
	}
	return encodeLine(shared, shrunk)
}

// prefixWithin returns the longest prefix of whole characters whose JSON
// encoding, without quotes, takes at most limit bytes.
func prefixWithin(s string, limit int) string {
	used := 0
	for i, r := range s {
		used += encodedLen(string(r))
		if used > limit {
			return s[:i]
		}
	}
	return s
}

// encodedLen returns the JSON-encoded size of s without its quotes.
func encodedLen(s string) int {
	return len(encodeValue(s)) - 2
}

func encodeLine(shared, custom []Field) []byte {
	var buf bytes.Buffer
	buf.WriteByte('{')
	writeFields(&buf, shared)
	if len(custom) > 0 {
		buf.WriteByte(',')
		writeFields(&buf, custom)
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeFields(buf *bytes.Buffer, fields []Field) {
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(encodeValue(f.Key))
		buf.WriteByte(':')
		buf.Write(encodeValue(f.Value))
	}
}

// encodeValue encodes v as JSON on one line, without HTML escaping.
func encodeValue(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		buf.Reset()
		_ = json.NewEncoder(&buf).Encode(fmt.Sprint(v))
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
