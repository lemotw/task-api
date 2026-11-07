package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	waitTimeout = 10 * time.Second
	childEnvKey = "TASK_API_TEST_RUN_MAIN"
)

// TestMain lets signal tests run the real main in a child process.
func TestMain(m *testing.M) {
	if os.Getenv(childEnvKey) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// syncBuffer is a concurrency-safe stdout.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// events parses every complete stdout line as a log event (LOG-1).
func events(t *testing.T, out string) []map[string]any {
	t.Helper()
	var evs []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stdout line is not JSON: %q", line)
		}
		evs = append(evs, ev)
	}
	return evs
}

func findEvents(t *testing.T, out, name string) []map[string]any {
	t.Helper()
	var found []map[string]any
	for _, ev := range events(t, out) {
		if ev["event"] == name {
			found = append(found, ev)
		}
	}
	return found
}

func hasEvent(out, name string) bool {
	return strings.Contains(out, `"event":"`+name+`"`)
}

// waitFor polls until cond holds, the process ends, or the timeout passes.
func waitFor(t *testing.T, what string, cond func() bool, ended <-chan struct{}, diag func() string) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for !cond() {
		select {
		case <-ended:
			if cond() {
				return
			}
			t.Fatalf("process ended before %s\n%s", what, diag())
		case <-deadline:
			t.Fatalf("timed out waiting for %s\n%s", what, diag())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type testServer struct {
	t      *testing.T
	out    *syncBuffer
	cancel context.CancelFunc
	done   chan struct{}
	code   int
	addr   string
}

func baseEnv(extra map[string]string) map[string]string {
	env := map[string]string{"PORT": "0"}
	maps.Copy(env, extra)
	return env
}

// runAsync starts run in a goroutine. The caller decides how to wait.
func runAsync(t *testing.T, env map[string]string, shutdownTimeout time.Duration) *testServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &testServer{t: t, out: &syncBuffer{}, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		s.code = run(ctx, runOptions{Getenv: envOf(env), Stdout: s.out, ShutdownTimeout: shutdownTimeout})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(waitTimeout):
			t.Error("run did not return during cleanup")
		}
	})
	return s
}

// startServer runs the server and waits until it listens.
func startServer(t *testing.T, env map[string]string, shutdownTimeout time.Duration) *testServer {
	t.Helper()
	s := runAsync(t, baseEnv(env), shutdownTimeout)
	waitFor(t, "server.started", func() bool { return hasEvent(s.out.String(), "server.started") },
		s.done, s.out.String)

	started := findEvents(t, s.out.String(), "server.started")[0]
	_, port, err := net.SplitHostPort(fmt.Sprint(started["addr"]))
	if err != nil || port == "0" {
		t.Fatalf("server.started addr = %v", started["addr"])
	}
	s.addr = net.JoinHostPort("127.0.0.1", port)
	return s
}

// stop cancels the server and returns its exit code.
func (s *testServer) stop() int {
	s.t.Helper()
	s.cancel()
	return s.wait()
}

func (s *testServer) wait() int {
	s.t.Helper()
	select {
	case <-s.done:
		return s.code
	case <-time.After(waitTimeout):
		s.t.Fatalf("run did not return\n%s", s.out.String())
		return 0
	}
}

var client = &http.Client{
	Timeout:   waitTimeout,
	Transport: &http.Transport{DisableKeepAlives: true},
}

func (s *testServer) request(method, path, body string) (int, string) {
	s.t.Helper()
	req, err := http.NewRequest(method, "http://"+s.addr+path, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func (s *testServer) create(name string) int {
	s.t.Helper()
	code, body := s.request(http.MethodPost, "/tasks", fmt.Sprintf(`{"name":%q,"status":0}`, name))
	if code != http.StatusCreated {
		s.t.Fatalf("create: %d %s", code, body)
	}
	var created struct{ ID int }
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		s.t.Fatal(err)
	}
	return created.ID
}

func (s *testServer) names() []string {
	s.t.Helper()
	code, body := s.request(http.MethodGet, "/tasks", "")
	if code != http.StatusOK {
		s.t.Fatalf("list: %d %s", code, body)
	}
	var list []struct{ Name string }
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		s.t.Fatal(err)
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		names = append(names, item.Name)
	}
	return names
}

// runToExit runs a server that is expected to stop by itself and returns its
// exit code and stdout.
func runToExit(t *testing.T, env map[string]string) (int, string) {
	t.Helper()
	s := runAsync(t, baseEnv(env), defaultShutdownTimeout)
	select {
	case <-s.done:
		return s.code, s.out.String()
	case <-time.After(waitTimeout):
		t.Fatalf("run kept serving\n%s", s.out.String())
		return 0, ""
	}
}

// partialPost opens a connection and sends a POST with half of its body. The
// caller sends the rest with finish.
type partialPost struct {
	conn net.Conn
	rest string
}

func startPartialPost(t *testing.T, s *testServer, name string) *partialPost {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"status":0}`, name)
	conn, err := net.Dial("tcp", s.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	half := len(body) / 2
	_, err = fmt.Fprintf(conn, "POST /tasks HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		len(body), body[:half])
	if err != nil {
		t.Fatal(err)
	}
	// Connections are accepted in order, so a completed request on a newer
	// connection proves this one was accepted too.
	s.names()
	return &partialPost{conn: conn, rest: body[half:]}
}

func (p *partialPost) finish(t *testing.T) int {
	t.Helper()
	if _, err := io.WriteString(p.conn, p.rest); err != nil {
		t.Fatal(err)
	}
	_ = p.conn.SetReadDeadline(time.Now().Add(waitTimeout))
	resp, err := http.ReadResponse(bufio.NewReader(p.conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// waitUntilClosedForNewConnections waits until shutdown has stopped listening.
func waitUntilClosedForNewConnections(t *testing.T, s *testServer) {
	t.Helper()
	waitFor(t, "the listener to close", func() bool {
		conn, err := net.DialTimeout("tcp", s.addr, 100*time.Millisecond)
		if err != nil {
			return true
		}
		conn.Close()
		return false
	}, s.done, s.out.String)
}

func eventIndex(t *testing.T, out, name string) int {
	t.Helper()
	for i, ev := range events(t, out) {
		if ev["event"] == name {
			return i
		}
	}
	t.Fatalf("no %s event in\n%s", name, out)
	return -1
}

func assertEqualNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func dataFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tasks.json")
}

func TestRun_ServesAPIAndStopsCleanly(t *testing.T) {
	s := startServer(t, nil, defaultShutdownTimeout)
	s.create("buy milk")
	assertEqualNames(t, s.names(), "buy milk")

	if code := s.stop(); code != 0 {
		t.Fatalf("exit code = %d\n%s", code, s.out.String())
	}
	out := s.out.String()
	started, request, stopped := eventIndex(t, out, "server.started"),
		eventIndex(t, out, "http.request"), eventIndex(t, out, "server.stopped")
	if !(started < request && request < stopped) {
		t.Errorf("event order: started %d, request %d, stopped %d", started, request, stopped)
	}
	evs := events(t, out)
	if last := evs[len(evs)-1]["event"]; last != "server.stopped" {
		t.Errorf("last event = %v, want server.stopped", last)
	}
}

func TestRun_ServesWebUI(t *testing.T) {
	s := startServer(t, nil, defaultShutdownTimeout)
	code, body := s.request(http.MethodGet, "/", "")
	if code != http.StatusOK || !strings.Contains(strings.ToLower(body), "<html") {
		t.Errorf("GET /: %d", code)
	}
}

func TestRun_WithoutDataFileWritesNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := startServer(t, nil, defaultShutdownTimeout)
	s.create("a")
	if code := s.stop(); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("working directory has %d entries", len(entries))
	}
}

func TestRun_PersistsAcrossRestart(t *testing.T) {
	for _, interval := range []string{"5s", "0"} {
		t.Run("interval "+interval, func(t *testing.T) {
			env := map[string]string{"DATA_FILE": dataFile(t), "DATA_FLUSH_INTERVAL": interval}
			first := startServer(t, env, defaultShutdownTimeout)
			first.create("a")
			first.create("b")
			if code := first.stop(); code != 0 {
				t.Fatalf("exit code = %d\n%s", code, first.out.String())
			}

			second := startServer(t, env, defaultShutdownTimeout)
			assertEqualNames(t, second.names(), "a", "b")
			if id := second.create("c"); id != 3 {
				t.Errorf("new id = %d, want 3", id)
			}
		})
	}
}

func TestRun_DeletedHighestIDIsNotReusedAfterRestart(t *testing.T) {
	env := map[string]string{"DATA_FILE": dataFile(t), "DATA_FLUSH_INTERVAL": "0"}
	first := startServer(t, env, defaultShutdownTimeout)
	first.create("a")
	high := first.create("b")
	if code, _ := first.request(http.MethodDelete, fmt.Sprintf("/tasks/%d", high), ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code := first.stop(); code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	second := startServer(t, env, defaultShutdownTimeout)
	if id := second.create("c"); id <= high {
		t.Errorf("new id = %d, reused or below %d", id, high)
	}
}

func TestRun_PeriodicFlushWritesWhileRunning(t *testing.T) {
	path := dataFile(t)
	s := startServer(t, map[string]string{"DATA_FILE": path, "DATA_FLUSH_INTERVAL": "20ms"}, defaultShutdownTimeout)
	s.create("periodic")
	waitFor(t, "the periodic flush", func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), "periodic")
	}, s.done, s.out.String)
}

func TestRun_InFlightRequestFinishesBeforeFinalFlush(t *testing.T) {
	env := map[string]string{"DATA_FILE": dataFile(t), "DATA_FLUSH_INTERVAL": "0"}
	s := startServer(t, env, waitTimeout)
	pending := startPartialPost(t, s, "in flight")

	s.cancel()
	waitUntilClosedForNewConnections(t, s)
	if code := pending.finish(t); code != http.StatusCreated {
		t.Fatalf("in-flight request: status %d", code)
	}
	if code := s.wait(); code != 0 {
		t.Fatalf("exit code = %d\n%s", code, s.out.String())
	}

	restarted := startServer(t, env, defaultShutdownTimeout)
	assertEqualNames(t, restarted.names(), "in flight")
}

func TestRun_ExternalSnapshotChangesAreNotLoaded(t *testing.T) {
	path := dataFile(t)
	s := startServer(t, map[string]string{"DATA_FILE": path, "DATA_FLUSH_INTERVAL": "0"}, defaultShutdownTimeout)
	s.create("a")
	external := `{"version":1,"next_id":9,"tasks":[{"id":8,"name":"external","status":0}]}`
	if err := os.WriteFile(path, []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	assertEqualNames(t, s.names(), "a")
}

func TestRun_PeriodicFlushFailureKeepsServingAndRetries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	s := startServer(t, map[string]string{"DATA_FILE": path, "DATA_FLUSH_INTERVAL": "20ms"}, defaultShutdownTimeout)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	s.create("a")
	waitFor(t, "snapshot.flush_failed", func() bool { return hasEvent(s.out.String(), "snapshot.flush_failed") },
		s.done, s.out.String)

	assertEqualNames(t, s.names(), "a")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the retried flush", func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), `"a"`)
	}, s.done, s.out.String)
	if code := s.stop(); code != 0 {
		t.Errorf("exit code = %d", code)
	}
}

func TestRun_FinalFlushFailureExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	s := startServer(t, map[string]string{
		"DATA_FILE": filepath.Join(dir, "tasks.json"), "DATA_FLUSH_INTERVAL": "0",
	}, defaultShutdownTimeout)
	s.create("unsaved")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if code := s.stop(); code == 0 {
		t.Fatalf("exit code = 0\n%s", s.out.String())
	}
	out := s.out.String()
	failed := findEvents(t, out, "snapshot.flush_failed")
	if len(failed) == 0 || failed[0]["level"] != "ERROR" {
		t.Errorf("flush_failed events = %v", failed)
	}
	if !hasEvent(out, "server.stopped") {
		t.Error("no server.stopped event")
	}
}

func TestRun_ShutdownTimeoutStillFlushesAndExitsNonZero(t *testing.T) {
	path := dataFile(t)
	s := startServer(t, map[string]string{"DATA_FILE": path, "DATA_FLUSH_INTERVAL": "0"}, 200*time.Millisecond)
	s.create("saved")
	startPartialPost(t, s, "never finished")

	if code := s.stop(); code == 0 {
		t.Fatalf("exit code = 0\n%s", s.out.String())
	}
	timeouts := findEvents(t, s.out.String(), "server.shutdown_timeout")
	if len(timeouts) != 1 || timeouts[0]["level"] != "WARN" || timeouts[0]["log_type"] != "debug" {
		t.Errorf("shutdown_timeout events = %v", timeouts)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "saved") {
		t.Errorf("final flush missing: %q, %v", data, err)
	}
}

func TestRun_InvalidConfigurationFailsToStart(t *testing.T) {
	cases := map[string]map[string]string{
		"log level":      {"LOG_LEVEL": "TRACE"},
		"log output":     {"LOG_OUTPUT": "udp://127.0.0.1:5140"},
		"flush interval": {"DATA_FLUSH_INTERVAL": "-1s"},
		"port":           {"PORT": "not-a-port"},
	}
	for label, env := range cases {
		t.Run(label, func(t *testing.T) {
			code, out := runToExit(t, env)
			if code == 0 {
				t.Fatalf("exit code = 0\n%s", out)
			}
			if hasEvent(out, "server.started") || !hasEvent(out, "server.start_failed") {
				t.Errorf("unexpected events\n%s", out)
			}
		})
	}
}

func TestRun_PortInUseFailsToStart(t *testing.T) {
	// Bind every interface like the server does: on macOS a 127.0.0.1 listener
	// does not stop another socket from binding the wildcard address.
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	_, port, _ := net.SplitHostPort(busy.Addr().String())

	code, out := runToExit(t, map[string]string{"PORT": port})
	if code == 0 {
		t.Fatalf("exit code = 0\n%s", out)
	}
	failed := findEvents(t, out, "server.start_failed")
	if len(failed) != 1 || failed[0]["level"] != "ERROR" || fmt.Sprint(failed[0]["error"]) == "" {
		t.Errorf("start_failed events = %v", failed)
	}
}

func TestRun_UnusableSnapshotFailsToStart(t *testing.T) {
	corrupt := func(t *testing.T) string {
		path := dataFile(t)
		if err := os.WriteFile(path, []byte(`{"version":1`), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := map[string]func(t *testing.T) string{
		"corrupt file":      corrupt,
		"path is directory": func(t *testing.T) string { return t.TempDir() },
		"missing directory": func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "missing", "tasks.json")
		},
	}
	for label, prepare := range cases {
		t.Run(label, func(t *testing.T) {
			path := prepare(t)
			before, _ := os.ReadFile(path)

			code, out := runToExit(t, map[string]string{"DATA_FILE": path})
			if code == 0 {
				t.Fatalf("exit code = 0\n%s", out)
			}
			if hasEvent(out, "server.started") || !hasEvent(out, "snapshot.load_failed") {
				t.Errorf("unexpected events\n%s", out)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Errorf("snapshot changed to %q", after)
			}
		})
	}
}

func TestMain_ShutsDownGracefullyOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			path := dataFile(t)
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), childEnvKey+"=1", "PORT=0",
				"DATA_FILE="+path, "DATA_FLUSH_INTERVAL=0")
			stdout, stderr := &syncBuffer{}, &syncBuffer{}
			cmd.Stdout, cmd.Stderr = stdout, stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan struct{})
			var waitErr error
			go func() {
				waitErr = cmd.Wait()
				close(exited)
			}()
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				<-exited
			})
			diag := func() string { return stdout.String() + stderr.String() }

			waitFor(t, "server.started", func() bool { return hasEvent(stdout.String(), "server.started") },
				exited, diag)
			started := findEvents(t, stdout.String(), "server.started")[0]
			_, port, _ := net.SplitHostPort(fmt.Sprint(started["addr"]))
			s := &testServer{t: t, addr: net.JoinHostPort("127.0.0.1", port)}
			s.create("signal")

			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-exited:
			case <-time.After(waitTimeout):
				t.Fatalf("process did not exit\n%s", diag())
			}
			if waitErr != nil {
				t.Fatalf("exit: %v\n%s", waitErr, diag())
			}
			if !hasEvent(stdout.String(), "server.stopped") {
				t.Errorf("no server.stopped event\n%s", diag())
			}
			if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "signal") {
				t.Errorf("snapshot missing the task: %q, %v", data, err)
			}
		})
	}
}
