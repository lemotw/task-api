package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

var errSink = errors.New("sink failed")

type fakeSink struct {
	mu       sync.Mutex
	lines    [][]byte
	writeErr error
	closeErr error
	closed   int
}

func (s *fakeSink) Write(line []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	s.lines = append(s.lines, append([]byte(nil), line...))
	return nil
}

func (s *fakeSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	return s.closeErr
}

func (s *fakeSink) raw() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.lines...)
}

func (s *fakeSink) setWriteErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeErr = err
}

func (s *fakeSink) events(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range s.raw() {
		out = append(out, decodeLine(t, line))
	}
	return out
}

func decodeLine(t *testing.T, line []byte) map[string]any {
	t.Helper()
	var ev map[string]any
	if err := json.Unmarshal(line, &ev); err != nil {
		t.Fatalf("invalid JSON %q: %v", line, err)
	}
	return ev
}

func fixedNow() time.Time {
	return time.Date(2026, 11, 7, 14, 54, 7, 123456789, time.FixedZone("TPE", 8*3600))
}

func newLogger(level Level) (*JSONLogger, *fakeSink) {
	sink := &fakeSink{}
	return New(Config{Level: level, Instance: "inst-1", Sink: sink, Now: fixedNow}), sink
}

func assertEnvelope(t *testing.T, ev map[string]any) {
	t.Helper()
	if ev["v"] != float64(1) {
		t.Errorf("v = %v", ev["v"])
	}
	if ev["service"] != "task-api" {
		t.Errorf("service = %v", ev["service"])
	}
	if ev["instance"] != "inst-1" {
		t.Errorf("instance = %v", ev["instance"])
	}
	if ev["time"] != "2026-11-07T06:54:07.123Z" {
		t.Errorf("time = %v", ev["time"])
	}
	for _, k := range []string{"log_type", "level", "seq", "event"} {
		if _, ok := ev[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
}

func TestEvent_IsOneJSONLineWithAllSharedFields(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("server.started")

	lines := sink.raw()
	if len(lines) != 1 {
		t.Fatalf("got %d lines", len(lines))
	}
	// Exactly one trailing newline, none inside.
	if !bytes.HasSuffix(lines[0], []byte("\n")) || bytes.HasSuffix(lines[0], []byte("\n\n")) {
		t.Errorf("event must end with exactly one newline: %q", lines[0])
	}
	if n := bytes.Count(bytes.TrimSuffix(lines[0], []byte("\n")), []byte("\n")); n != 0 {
		t.Errorf("event spans multiple lines: %q", lines[0])
	}
	assertEnvelope(t, decodeLine(t, lines[0]))
}

func TestStream_ConsecutiveEventsAreSeparateJSONLines(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Level: LevelDebug, Instance: "inst-1", Sink: NewWriterSink(&buf), Now: fixedNow})
	l.Info("first")
	l.Access(AccessEvent{Method: "GET", Route: "/tasks", Path: "/tasks", Status: 200})

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("stream does not end with a newline: %q", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out)
	}
	wantEvents := []string{"first", "http.request"}
	for i, line := range lines {
		ev := decodeLine(t, []byte(line))
		assertEnvelope(t, ev)
		if ev["event"] != wantEvents[i] {
			t.Errorf("line %d event = %v, want %s", i, ev["event"], wantEvents[i])
		}
	}
}

func TestSeq_StartsAtOneAndIncrements(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("a")
	l.Info("b")
	l.Info("c")
	evs := sink.events(t)
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3", len(evs))
	}
	for i, ev := range evs {
		if ev["seq"] != float64(i+1) {
			t.Errorf("event %d seq = %v", i, ev["seq"])
		}
	}
}

func TestSeq_FilteredEventsDoNotConsumeNumbers(t *testing.T) {
	l, sink := newLogger(LevelWarn)
	l.Warn("a")
	l.Debug("filtered")
	l.Info("filtered")
	l.Error("b")

	evs := sink.events(t)
	if len(evs) != 2 {
		t.Fatalf("got %d events", len(evs))
	}
	if evs[0]["seq"] != float64(1) || evs[1]["seq"] != float64(2) {
		t.Errorf("seq = %v, %v", evs[0]["seq"], evs[1]["seq"])
	}
}

func TestSeq_SharedBetweenAccessAndDebug(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("a")
	l.Access(AccessEvent{Method: "GET", Route: "/tasks", Path: "/tasks", Status: 200})
	l.Info("b")

	evs := sink.events(t)
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3", len(evs))
	}
	for i, ev := range evs {
		if ev["seq"] != float64(i+1) {
			t.Errorf("event %d seq = %v", i, ev["seq"])
		}
		if ev["instance"] != "inst-1" {
			t.Errorf("event %d instance = %v", i, ev["instance"])
		}
	}
}

func TestCustomFields_AreAtTopLevel(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("server.started", Field{Key: "addr", Value: ":8080"})
	if got := sink.events(t)[0]["addr"]; got != ":8080" {
		t.Errorf("addr = %v", got)
	}
}

func TestCustomFields_CollisionKeepsSharedField(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("real",
		Field{Key: "event", Value: "fake"},
		Field{Key: "level", Value: "fake"},
		Field{Key: "seq", Value: 999},
	)
	ev := sink.events(t)[0]
	if ev["event"] != "real" || ev["level"] != "INFO" || ev["seq"] != float64(1) {
		t.Errorf("shared fields overwritten: %v", ev)
	}
}

func TestCustomFields_SpecialCharactersStayOnOneLine(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	value := "line1\nline2\"quoted\""
	l.Error("failed", Field{Key: "error", Value: value})

	line := sink.raw()[0]
	if bytes.Contains(bytes.TrimRight(line, "\n"), []byte("\n")) {
		t.Errorf("raw newline in %q", line)
	}
	if got := decodeLine(t, line)["error"]; got != value {
		t.Errorf("error = %v", got)
	}
}

func TestAccess_WritesAllFields(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Access(AccessEvent{
		Method: "PUT", Route: "/tasks/{id}", Path: "/tasks/1",
		Status: 200, DurationMS: 7, Bytes: 38,
	})
	ev := sink.events(t)[0]
	assertEnvelope(t, ev)
	want := map[string]any{
		"log_type": "access", "event": "http.request", "method": "PUT",
		"route": "/tasks/{id}", "path": "/tasks/1", "status": float64(200),
		"duration_ms": float64(7), "bytes": float64(38),
	}
	for k, v := range want {
		if ev[k] != v {
			t.Errorf("%s = %v, want %v", k, ev[k], v)
		}
	}
}

func TestAccess_LevelFollowsStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{200, "INFO"}, {204, "INFO"}, {400, "INFO"}, {404, "INFO"},
		{500, "ERROR"}, {503, "ERROR"},
	}
	for _, c := range cases {
		l, sink := newLogger(LevelDebug)
		l.Access(AccessEvent{Status: c.status})
		if got := sink.events(t)[0]["level"]; got != c.want {
			t.Errorf("status %d level = %v, want %s", c.status, got, c.want)
		}
	}
}

func TestAccess_NotFilteredByLogLevel(t *testing.T) {
	l, sink := newLogger(LevelError)
	l.Access(AccessEvent{Status: 200})
	if n := len(sink.raw()); n != 1 {
		t.Fatalf("got %d events", n)
	}
}

func TestDebugEvents_UseDebugLogTypeAndUppercaseLevel(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Debug("a")
	l.Info("b")
	l.Warn("c")
	l.Error("d")
	want := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	evs := sink.events(t)
	if len(evs) != len(want) {
		t.Fatalf("got %d events", len(evs))
	}
	for i, ev := range evs {
		if ev["log_type"] != "debug" {
			t.Errorf("event %d log_type = %v", i, ev["log_type"])
		}
		if ev["level"] != want[i] {
			t.Errorf("event %d level = %v, want %s", i, ev["level"], want[i])
		}
	}
}

func TestLogLevel_FiltersBelowConfiguredLevel(t *testing.T) {
	levels := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	for _, configured := range levels {
		for _, emitted := range levels {
			l, sink := newLogger(configured)
			switch emitted {
			case LevelDebug:
				l.Debug("e")
			case LevelInfo:
				l.Info("e")
			case LevelWarn:
				l.Warn("e")
			case LevelError:
				l.Error("e")
			}
			want := 0
			if emitted >= configured {
				want = 1
			}
			if got := len(sink.raw()); got != want {
				t.Errorf("configured %d, emitted %d: got %d events", configured, emitted, got)
			}
		}
	}
}

func TestLevel_String(t *testing.T) {
	cases := map[Level]string{
		LevelDebug: "DEBUG", LevelInfo: "INFO", LevelWarn: "WARN", LevelError: "ERROR",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("Level(%d) = %q, want %q", level, got, want)
		}
	}
}

func TestParseLevel_AcceptsAnyCase(t *testing.T) {
	cases := map[string]Level{
		"DEBUG": LevelDebug, "debug": LevelDebug, "Info": LevelInfo,
		"WARN": LevelWarn, "warn": LevelWarn, "error": LevelError,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", in, got, err)
		}
	}
}

func TestParseLevel_RejectsUnknownValues(t *testing.T) {
	for _, in := range []string{"", "TRACE", "warning", " INFO"} {
		if _, err := ParseLevel(in); err == nil {
			t.Errorf("ParseLevel(%q) returned no error", in)
		}
	}
}

func TestConcurrentEvents_AreCompleteAndOrdered(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	const workers, perWorker = 8, 100

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				l.Info("a")
				l.Access(AccessEvent{Status: 200})
			}
		}()
	}
	wg.Wait()

	evs := sink.events(t)
	if want := workers * perWorker * 2; len(evs) != want {
		t.Fatalf("got %d events, want %d", len(evs), want)
	}
	for i, ev := range evs {
		if ev["seq"] != float64(i+1) {
			t.Fatalf("line %d seq = %v", i, ev["seq"])
		}
	}
}

func TestSinkWriteFailure_DoesNotStopLaterEvents(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	sink.setWriteErr(errSink)
	l.Info("lost")
	l.Access(AccessEvent{Status: 200})

	sink.setWriteErr(nil)
	l.Info("kept")
	evs := sink.events(t)
	if len(evs) != 1 || evs[0]["event"] != "kept" {
		t.Fatalf("events = %v", evs)
	}
}

// wantEnvelope is the exact value of every per-event shared field.
type wantEnvelope struct {
	logType string
	level   string
	seq     float64
	event   string
}

func assertTruncated(t *testing.T, line []byte, want wantEnvelope) map[string]any {
	t.Helper()
	if len(line) > 1400 {
		t.Errorf("event is %d bytes", len(line))
	}
	if !utf8.Valid(line) {
		t.Error("event is not valid UTF-8")
	}
	ev := decodeLine(t, line)
	if ev["truncated"] != true {
		t.Errorf("truncated = %v", ev["truncated"])
	}
	assertEnvelope(t, ev)
	if ev["log_type"] != want.logType {
		t.Errorf("log_type = %v, want %s", ev["log_type"], want.logType)
	}
	if ev["level"] != want.level {
		t.Errorf("level = %v, want %s", ev["level"], want.level)
	}
	if ev["seq"] != want.seq {
		t.Errorf("seq = %v, want %v", ev["seq"], want.seq)
	}
	if ev["event"] != want.event {
		t.Errorf("event = %v, want %s", ev["event"], want.event)
	}
	return ev
}

func TestTruncation_LongPath(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Access(AccessEvent{Method: "GET", Route: "unmatched", Path: "/" + strings.Repeat("a", 5000), Status: 404})

	ev := assertTruncated(t, sink.raw()[0],
		wantEnvelope{logType: "access", level: "INFO", seq: 1, event: "http.request"})
	if ev["status"] != float64(404) || ev["method"] != "GET" || ev["route"] != "unmatched" {
		t.Errorf("fixed fields changed: %v", ev)
	}
}

func TestTruncation_MeasuresEncodedSize(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	nasty := strings.Repeat("\"\\\n", 2000)
	l.Error("failed", Field{Key: "path", Value: nasty}, Field{Key: "error", Value: nasty})
	assertTruncated(t, sink.raw()[0],
		wantEnvelope{logType: "debug", level: "ERROR", seq: 1, event: "failed"})
}

func TestTruncation_KeepsMultibyteCharactersValid(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Error("failed", Field{Key: "error", Value: strings.Repeat("任", 1000)})
	ev := assertTruncated(t, sink.raw()[0],
		wantEnvelope{logType: "debug", level: "ERROR", seq: 1, event: "failed"})
	got, ok := ev["error"].(string)
	if !ok {
		t.Fatalf("error = %v", ev["error"])
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Error("error contains a replacement character")
	}
	// What remains must be an unbroken prefix of the original value.
	if n := utf8.RuneCountInString(got); n == 0 || got != strings.Repeat("任", n) {
		t.Errorf("error is not a whole-character prefix: %d runes", n)
	}
}

func TestShortEvent_IsNotMarkedTruncated(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	l.Info("short")
	if _, ok := sink.events(t)[0]["truncated"]; ok {
		t.Error("short event is marked truncated")
	}
}

func TestClose_ClosesSink(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	if err := l.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if sink.closed != 1 {
		t.Errorf("sink closed %d times", sink.closed)
	}
}

func TestClose_ReturnsSinkError(t *testing.T) {
	l, sink := newLogger(LevelDebug)
	sink.closeErr = errSink
	if err := l.Close(); !errors.Is(err, errSink) {
		t.Fatalf("Close = %v", err)
	}
}
