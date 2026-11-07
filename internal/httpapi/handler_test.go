package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/lemotw/task-api/internal/logging"
	"github.com/lemotw/task-api/internal/logging/logtest"
	"github.com/lemotw/task-api/internal/memory"
	"github.com/lemotw/task-api/internal/task"
)

const (
	oneMiB    = 1 << 20
	validBody = `{"name":"a","status":0}`
)

type env struct {
	handler http.Handler
	log     *logtest.Recorder
}

func newEnv(pages map[string]http.Handler) *env {
	log := &logtest.Recorder{}
	return &env{
		handler: NewHandler(Deps{Tasks: task.NewService(memory.New()), Log: log, Pages: pages}),
		log:     log,
	}
}

// uiPaths are the paths the web UI owns (UI-7).
var uiPaths = []string{"/", "/test"}

// fakePages returns a page per UI path that answers 418 and names its path.
func fakePages() map[string]http.Handler {
	pages := make(map[string]http.Handler, len(uiPaths))
	for _, path := range uiPaths {
		pages[path] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Fake-UI", path)
			w.WriteHeader(http.StatusTeapot)
		})
	}
	return pages
}

// forEachUI runs the API contract with the web UI disabled and enabled (UI-7).
func forEachUI(t *testing.T, fn func(t *testing.T, e *env)) {
	t.Helper()
	t.Run("ui off", func(t *testing.T) { fn(t, newEnv(nil)) })
	t.Run("ui on", func(t *testing.T) { fn(t, newEnv(fakePages())) })
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (e *env) do(method, target, body string) *httptest.ResponseRecorder {
	return serve(e.handler, httptest.NewRequest(method, target, strings.NewReader(body)))
}

// doChunked sends a body of unknown length: no Content-Length is available.
func (e *env) doChunked(method, target, body string) *httptest.ResponseRecorder {
	reader := struct{ io.Reader }{strings.NewReader(body)}
	return serve(e.handler, httptest.NewRequest(method, target, reader))
}

func (e *env) seed(t *testing.T) int {
	t.Helper()
	rec := e.do(http.MethodPost, "/tasks", `{"name":"seed","status":0}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed: status %d, body %s", rec.Code, rec.Body)
	}
	return int(decodeTask(t, rec.Body.Bytes())["id"].(float64))
}

func decodeTask(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", body, err)
	}
	return got
}

func assertTask(t *testing.T, rec *httptest.ResponseRecorder, id int, name string, status int) {
	t.Helper()
	assertJSONType(t, rec)
	got := decodeTask(t, rec.Body.Bytes())
	want := map[string]any{"id": float64(id), "name": name, "status": float64(status)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("task = %v, want %v", got, want)
	}
}

func assertJSONType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, wantStatus, rec.Body)
	}
	assertJSONType(t, rec)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}
	msg, ok := body["error"].(string)
	if !ok || msg == "" || len(body) != 1 {
		t.Errorf("error body = %v", body)
	}
}

// assertNoSideEffects checks that a failed request changed no task and used no
// id (DOM-11). wantNextID is the id the next created task must receive.
func assertNoSideEffects(t *testing.T, e *env, wantList string, wantNextID int) {
	t.Helper()
	if got := e.do(http.MethodGet, "/tasks", "").Body.String(); got != wantList {
		t.Errorf("list changed\n got: %s\nwant: %s", got, wantList)
	}
	rec := e.do(http.MethodPost, "/tasks", validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create after failure: status %d", rec.Code)
	}
	if got := decodeTask(t, rec.Body.Bytes())["id"]; got != float64(wantNextID) {
		t.Errorf("next id = %v, want %d", got, wantNextID)
	}
}

func paddedBody(size int) string {
	return validBody + strings.Repeat(" ", size-len(validBody))
}

func TestList_EmptyReturnsEmptyArray(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		rec := e.do(http.MethodGet, "/tasks", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		assertJSONType(t, rec)
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Errorf("body = %q", got)
		}
	})
}

func TestList_ReturnsTasksOrderedByID(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		for i := range 5 {
			e.do(http.MethodPost, "/tasks", fmt.Sprintf(`{"name":"t%d","status":0}`, i))
		}
		rec := e.do(http.MethodGet, "/tasks", "")
		var got []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("got %d tasks", len(got))
		}
		for i, item := range got {
			want := map[string]any{"id": float64(i + 1), "name": fmt.Sprintf("t%d", i), "status": float64(0)}
			if !reflect.DeepEqual(item, want) {
				t.Errorf("position %d = %v", i, item)
			}
		}
	})
}

func TestCreate_ReturnsCreatedTaskAndLocation(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		rec := e.do(http.MethodPost, "/tasks", `{"name":"buy milk","status":1}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Location"); got != "/tasks/1" {
			t.Errorf("Location = %q", got)
		}
		assertTask(t, rec, 1, "buy milk", 1)
	})
}

func TestCreate_KeepsNameUnchanged(t *testing.T) {
	e := newEnv(nil)
	rec := e.do(http.MethodPost, "/tasks", `{"name":"  buy milk  ","status":0}`)
	assertTask(t, rec, 1, "  buy milk  ", 0)
}

func TestCreate_DoesNotDependOnContentType(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		e := newEnv(nil)
		req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(validBody))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if rec := serve(e.handler, req); rec.Code != http.StatusCreated {
			t.Errorf("Content-Type %q: status %d", contentType, rec.Code)
		}
	}
}

func TestBody_FieldNamesIgnoreCase(t *testing.T) {
	e := newEnv(nil)
	rec := e.do(http.MethodPost, "/tasks", `{"NAME":"a","Status":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	assertTask(t, rec, 1, "a", 1)
}

func TestBody_TrailingWhitespaceIsAccepted(t *testing.T) {
	for _, suffix := range []string{" ", "\n", "\r\n\t "} {
		e := newEnv(nil)
		if rec := e.do(http.MethodPost, "/tasks", validBody+suffix); rec.Code != http.StatusCreated {
			t.Errorf("suffix %q: status %d, body %s", suffix, rec.Code, rec.Body)
		}
	}
}

func TestBody_ExactlyOneMiBIsAccepted(t *testing.T) {
	e := newEnv(nil)
	if rec := e.do(http.MethodPost, "/tasks", paddedBody(oneMiB)); rec.Code != http.StatusCreated {
		t.Errorf("POST: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodPut, "/tasks/1", paddedBody(oneMiB)); rec.Code != http.StatusOK {
		t.Errorf("PUT: status %d, body %s", rec.Code, rec.Body)
	}
}

func TestBody_OverOneMiBIsRejected(t *testing.T) {
	cases := map[string]func(e *env, method, target, body string) *httptest.ResponseRecorder{
		"known length":   (*env).do,
		"unknown length": (*env).doChunked,
	}
	for label, send := range cases {
		t.Run(label, func(t *testing.T) {
			e := newEnv(nil)
			id := e.seed(t)
			before := e.do(http.MethodGet, "/tasks", "").Body.String()

			assertError(t, send(e, http.MethodPost, "/tasks", paddedBody(oneMiB+1)), http.StatusRequestEntityTooLarge)
			assertError(t, send(e, http.MethodPut, "/tasks/"+strconv.Itoa(id), paddedBody(oneMiB+1)), http.StatusRequestEntityTooLarge)
			assertNoSideEffects(t, e, before, id+1)
		})
	}
}

// The first byte is already invalid JSON, so a handler that parses while reading
// would answer 400 before it could see the size (API-7).
func TestBody_OverOneMiBWinsOverImmediatelyInvalidJSON(t *testing.T) {
	body := "!" + strings.Repeat(" ", oneMiB)
	senders := map[string]func(e *env, method, target, body string) *httptest.ResponseRecorder{
		"known length":   (*env).do,
		"unknown length": (*env).doChunked,
	}
	requests := map[string]struct{ method, target string }{
		"POST": {http.MethodPost, "/tasks"},
		"PUT":  {http.MethodPut, "/tasks/1"},
	}
	for sendLabel, send := range senders {
		for reqLabel, req := range requests {
			t.Run(sendLabel+"/"+reqLabel, func(t *testing.T) {
				e := newEnv(nil)
				e.seed(t)
				assertError(t, send(e, req.method, req.target, body), http.StatusRequestEntityTooLarge)
			})
		}
	}
}

func TestUpdate_ReplacesTask(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		id := e.seed(t)
		rec := e.do(http.MethodPut, "/tasks/"+strconv.Itoa(id), `{"name":"new","status":1}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		assertTask(t, rec, id, "new", 1)
	})
}

func TestDelete_ReturnsNoContent(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		id := e.seed(t)
		rec := e.do(http.MethodDelete, "/tasks/"+strconv.Itoa(id), "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q", rec.Body.String())
		}
		if got := strings.TrimSpace(e.do(http.MethodGet, "/tasks", "").Body.String()); got != "[]" {
			t.Errorf("list = %q", got)
		}
	})
}

func TestCRUD_FullFlow(t *testing.T) {
	forEachUI(t, func(t *testing.T, e *env) {
		assertTask(t, e.do(http.MethodPost, "/tasks", `{"name":"a","status":0}`), 1, "a", 0)
		assertTask(t, e.do(http.MethodPost, "/tasks", `{"name":"a","status":0}`), 2, "a", 0)
		assertTask(t, e.do(http.MethodPut, "/tasks/1", `{"name":"b","status":1}`), 1, "b", 1)

		if rec := e.do(http.MethodDelete, "/tasks/2", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: status %d", rec.Code)
		}
		var list []map[string]any
		if err := json.Unmarshal(e.do(http.MethodGet, "/tasks", "").Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		want := []map[string]any{{"id": float64(1), "name": "b", "status": float64(1)}}
		if !reflect.DeepEqual(list, want) {
			t.Errorf("list = %v", list)
		}
		assertTask(t, e.do(http.MethodPost, "/tasks", `{"name":"c","status":0}`), 3, "c", 0)
	})
}

func TestBody_InvalidIsRejectedWithoutSideEffects(t *testing.T) {
	bodies := map[string]string{
		"empty":            ``,
		"broken JSON":      `{`,
		"array":            `[]`,
		"string":           `"x"`,
		"null":             `null`,
		"second value":     validBody + `{}`,
		"trailing garbage": validBody + ` x`,
		"missing name":     `{"status":0}`,
		"missing status":   `{"name":"a"}`,
		"null name":        `{"name":null,"status":0}`,
		"null status":      `{"name":"a","status":null}`,
		"id field":         `{"name":"a","status":0,"id":9}`,
		"unknown field":    `{"name":"a","status":0,"foo":1}`,
		"status string":    `{"name":"a","status":"1"}`,
		"status fraction":  `{"name":"a","status":0.5}`,
		"status exponent":  `{"name":"a","status":1e0}`,
		"status 1.0":       `{"name":"a","status":1.0}`,
		"name number":      `{"name":1,"status":0}`,
		"status 2":         `{"name":"a","status":2}`,
		"status -1":        `{"name":"a","status":-1}`,
		"blank name":       `{"name":"   ","status":0}`,
		"long name":        `{"name":"` + strings.Repeat("任", 256) + `","status":0}`,
	}
	requests := map[string]struct{ method, target string }{
		"POST": {http.MethodPost, "/tasks"},
		"PUT":  {http.MethodPut, "/tasks/1"},
	}
	for reqLabel, req := range requests {
		for label, body := range bodies {
			t.Run(reqLabel+"/"+label, func(t *testing.T) {
				e := newEnv(nil)
				id := e.seed(t)
				before := e.do(http.MethodGet, "/tasks", "").Body.String()

				assertError(t, e.do(req.method, req.target, body), http.StatusBadRequest)
				assertNoSideEffects(t, e, before, id+1)
			})
		}
	}
}

func TestID_InvalidFormatIsRejected(t *testing.T) {
	ids := []string{"abc", "0", "01", "+1", "-1", "1.5", "99999999999999999999999"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			e := newEnv(nil)
			e.seed(t)
			assertError(t, e.do(http.MethodPut, "/tasks/"+id, validBody), http.StatusBadRequest)
			assertError(t, e.do(http.MethodDelete, "/tasks/"+id, ""), http.StatusBadRequest)
		})
	}
}

func TestUnknownID_ReturnsNotFoundWithoutCreating(t *testing.T) {
	e := newEnv(nil)
	id := e.seed(t)
	before := e.do(http.MethodGet, "/tasks", "").Body.String()

	assertError(t, e.do(http.MethodPut, "/tasks/99", validBody), http.StatusNotFound)
	assertError(t, e.do(http.MethodDelete, "/tasks/99", ""), http.StatusNotFound)
	assertNoSideEffects(t, e, before, id+1)
}

func TestMethodNotAllowed_ReturnsAllowHeader(t *testing.T) {
	cases := []struct{ method, target, allow string }{
		{http.MethodPatch, "/tasks", "GET, POST"},
		{http.MethodPut, "/tasks", "GET, POST"},
		{http.MethodDelete, "/tasks", "GET, POST"},
		{http.MethodOptions, "/tasks", "GET, POST"},
		{http.MethodHead, "/tasks", "GET, POST"},
		{http.MethodGet, "/tasks/1", "PUT, DELETE"},
		{http.MethodPost, "/tasks/1", "PUT, DELETE"},
		{http.MethodPatch, "/tasks/1", "PUT, DELETE"},
		{http.MethodHead, "/tasks/1", "PUT, DELETE"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.target, func(t *testing.T) {
			e := newEnv(nil)
			e.seed(t)
			rec := e.do(c.method, c.target, "")
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != c.allow {
				t.Errorf("Allow = %q, want %q", got, c.allow)
			}
			assertJSONType(t, rec)
			if c.method != http.MethodHead { // a real server sends no body for HEAD
				assertError(t, rec, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestUnknownPaths_ReturnNotFound(t *testing.T) {
	cases := []struct{ method, target string }{
		{http.MethodGet, "/nope"},
		{http.MethodPost, "/nope"},
		{http.MethodPut, "/tasks/abc/def"},
		{http.MethodDelete, "/tasks/1/2"},
		{http.MethodGet, "/tasks/1/2"},
		{http.MethodGet, "/tasksx"},
		{http.MethodGet, "/test/"},
		{http.MethodGet, "/test/x"},
		{http.MethodGet, "/tests"},
		{http.MethodGet, "/index.html"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.target, func(t *testing.T) {
			forEachUI(t, func(t *testing.T, e *env) {
				rec := e.do(c.method, c.target, "")
				assertError(t, rec, http.StatusNotFound)
				if rec.Header().Get("X-Fake-UI") != "" {
					t.Error("request was handled by the UI")
				}
			})
		})
	}
}

func TestUIPaths_WithoutUIReturnNotFound(t *testing.T) {
	for _, path := range uiPaths {
		assertError(t, newEnv(nil).do(http.MethodGet, path, ""), http.StatusNotFound)
	}
}

func TestUIPaths_AreDelegatedForAllMethods(t *testing.T) {
	for _, path := range uiPaths {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			rec := newEnv(fakePages()).do(method, path, "")
			if rec.Code != http.StatusTeapot || rec.Header().Get("X-Fake-UI") != path {
				t.Errorf("%s %s: status %d, page %q", method, path, rec.Code, rec.Header().Get("X-Fake-UI"))
			}
		}
	}
}

func TestCheckOrder(t *testing.T) {
	oversized := strings.Repeat(" ", oneMiB+1)
	invalidOversized := "{" + oversized
	cases := []struct {
		label, method, target, body string
		want                        int
	}{
		{"path before method", http.MethodDelete, "/nope", "", http.StatusNotFound},
		{"id before method", http.MethodGet, "/tasks/abc", "", http.StatusBadRequest},
		{"id before size", http.MethodPut, "/tasks/abc", oversized, http.StatusBadRequest},
		{"method before size", http.MethodPatch, "/tasks", oversized, http.StatusMethodNotAllowed},
		{"method before size with id", http.MethodGet, "/tasks/1", oversized, http.StatusMethodNotAllowed},
		{"size before format", http.MethodPost, "/tasks", invalidOversized, http.StatusRequestEntityTooLarge},
		{"size before existence", http.MethodPut, "/tasks/99", paddedBody(oneMiB + 1), http.StatusRequestEntityTooLarge},
		{"format before existence", http.MethodPut, "/tasks/99", `{`, http.StatusBadRequest},
		{"validation before existence", http.MethodPut, "/tasks/99", `{"name":"a","status":2}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			e := newEnv(nil)
			e.seed(t)
			assertError(t, e.do(c.method, c.target, c.body), c.want)
		})
	}
}

type fakeService struct {
	result task.Task
	err    error
}

func (s fakeService) List() []task.Task { return nil }

func (s fakeService) Create(string, int) (task.Task, error) { return s.result, s.err }

func (s fakeService) Update(int, string, int) (task.Task, error) { return s.result, s.err }

func (s fakeService) Delete(int) error { return s.err }

func TestInternalError_HidesDetailsAndLogsThem(t *testing.T) {
	cases := []struct{ method, target, route, body string }{
		{http.MethodPost, "/tasks", "/tasks", validBody},
		{http.MethodPut, "/tasks/1", "/tasks/{id}", validBody},
		{http.MethodDelete, "/tasks/1", "/tasks/{id}", ""},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			log := &logtest.Recorder{}
			h := NewHandler(Deps{
				Tasks: fakeService{err: errors.New("db password secret-value")},
				Log:   log,
			})
			req := httptest.NewRequest(c.method, c.target, strings.NewReader(c.body))
			rec := serve(h, req)

			assertError(t, rec, http.StatusInternalServerError)
			if strings.Contains(rec.Body.String(), "secret-value") {
				t.Errorf("response leaks details: %s", rec.Body)
			}

			entries := log.Find("http.internal_error")
			if len(entries) != 1 {
				t.Fatalf("got %d internal error events", len(entries))
			}
			e := entries[0]
			if e.Level != logging.LevelError {
				t.Errorf("level = %v", e.Level)
			}
			if got := fmt.Sprint(e.Fields["error"]); !strings.Contains(got, "secret-value") {
				t.Errorf("error = %q", got)
			}
			if got := fmt.Sprint(e.Fields["route"]); got != c.route {
				t.Errorf("route = %q, want %q", got, c.route)
			}
			if got := fmt.Sprint(e.Fields["path"]); got != c.target {
				t.Errorf("path = %q, want %q", got, c.target)
			}

			access := log.AccessEvents()
			if len(access) != 1 || access[0].Status != http.StatusInternalServerError {
				t.Errorf("access = %+v", access)
			}
		})
	}
}

func TestAccessLog_RecordsEveryRequestAccurately(t *testing.T) {
	cases := []struct {
		label, method, target, body string
		route                       string
		status                      int
	}{
		{"list", http.MethodGet, "/tasks", "", "/tasks", 200},
		{"create", http.MethodPost, "/tasks", validBody, "/tasks", 201},
		{"update", http.MethodPut, "/tasks/1", validBody, "/tasks/{id}", 200},
		{"delete", http.MethodDelete, "/tasks/1", "", "/tasks/{id}", 204},
		{"invalid body", http.MethodPost, "/tasks", `{`, "/tasks", 400},
		{"invalid id", http.MethodPut, "/tasks/abc", validBody, "/tasks/{id}", 400},
		{"unknown id", http.MethodDelete, "/tasks/99", "", "/tasks/{id}", 404},
		{"wrong method", http.MethodPatch, "/tasks", "", "/tasks", 405},
		{"oversized", http.MethodPost, "/tasks", paddedBody(oneMiB + 1), "/tasks", 413},
		{"unknown path", http.MethodGet, "/nope", "", "unmatched", 404},
		{"too many segments", http.MethodGet, "/tasks/1/2", "", "unmatched", 404},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			e := newEnv(nil)
			e.seed(t)
			before := len(e.log.AccessEvents())

			rec := e.do(c.method, c.target, c.body)

			events := e.log.AccessEvents()
			if len(events) != before+1 {
				t.Fatalf("got %d access events for one request", len(events)-before)
			}
			got := events[len(events)-1]
			if got.Method != c.method || got.Route != c.route || got.Path != c.target || got.Status != c.status {
				t.Errorf("event = %+v", got)
			}
			if rec.Code != c.status {
				t.Errorf("response status = %d, want %d", rec.Code, c.status)
			}
			if got.Bytes != int64(rec.Body.Len()) {
				t.Errorf("bytes = %d, want %d", got.Bytes, rec.Body.Len())
			}
			if got.DurationMS < 0 {
				t.Errorf("duration = %d", got.DurationMS)
			}
		})
	}
}

// A real server sends no body for HEAD, so the log must not count one. The
// ResponseRecorder keeps HEAD bodies, so this needs a real server.
func TestAccessLog_HeadOnRealServerHasZeroBytes(t *testing.T) {
	e := newEnv(nil)
	server := httptest.NewServer(e.handler)
	defer server.Close()

	for _, target := range []string{"/tasks", "/tasks/1", "/nope"} {
		req, err := http.NewRequest(http.MethodHead, server.URL+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		events := e.log.AccessEvents()
		got := events[len(events)-1]
		if got.Method != http.MethodHead || got.Status != resp.StatusCode {
			t.Fatalf("%s: event = %+v, response status %d", target, got, resp.StatusCode)
		}
		if got.Bytes != int64(len(body)) || got.Bytes != 0 {
			t.Errorf("%s: logged %d bytes, sent %d", target, got.Bytes, len(body))
		}
	}
}

// For other methods the logged size matches what the client received.
func TestAccessLog_BytesMatchRealServerResponse(t *testing.T) {
	e := newEnv(nil)
	server := httptest.NewServer(e.handler)
	defer server.Close()

	resp, err := server.Client().Post(server.URL+"/tasks", "application/json", strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	events := e.log.AccessEvents()
	if got := events[len(events)-1]; got.Bytes != int64(len(body)) || got.Bytes == 0 {
		t.Errorf("logged %d bytes, sent %d", got.Bytes, len(body))
	}
}

func TestAccessLog_DeleteHasZeroBytes(t *testing.T) {
	e := newEnv(nil)
	id := e.seed(t)
	e.do(http.MethodDelete, "/tasks/"+strconv.Itoa(id), "")
	events := e.log.AccessEvents()
	if got := events[len(events)-1]; got.Status != 204 || got.Bytes != 0 {
		t.Errorf("event = %+v", got)
	}
}

func TestAccessLog_UIRoutesUseTheirPath(t *testing.T) {
	for _, path := range uiPaths {
		e := newEnv(fakePages())
		e.do(http.MethodGet, path, "")
		events := e.log.AccessEvents()
		if len(events) != 1 || events[0].Route != path || events[0].Status != http.StatusTeapot {
			t.Errorf("%s: events = %+v", path, events)
		}
	}
}

func TestLogs_ContainNoBodiesHeadersOrQuery(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(logging.Config{
		Level: logging.LevelDebug, Instance: "i", Sink: logging.NewWriterSink(&buf),
	})
	h := NewHandler(Deps{
		Tasks: fakeService{result: task.Task{ID: 1, Name: "response-marker"}},
		Log:   logger,
	})
	req := httptest.NewRequest(http.MethodPost, "/tasks?q=query-marker",
		strings.NewReader(`{"name":"request-marker","status":0}`))
	req.Header.Set("X-Secret", "header-marker")
	req.Header.Set("Authorization", "Bearer auth-marker")

	rec := serve(h, req)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "response-marker") {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}

	out := buf.String()
	if !strings.Contains(out, "http.request") {
		t.Fatalf("no access event in %q", out)
	}
	for _, marker := range []string{"request-marker", "response-marker", "query-marker", "header-marker", "auth-marker"} {
		if strings.Contains(out, marker) {
			t.Errorf("log contains %q: %s", marker, out)
		}
	}
}

type failingSink struct{}

func (failingSink) Write([]byte) error { return errors.New("sink failed") }

func (failingSink) Close() error { return nil }

func TestLoggingFailure_DoesNotAffectResponses(t *testing.T) {
	logger := logging.New(logging.Config{Level: logging.LevelDebug, Instance: "i", Sink: failingSink{}})
	h := NewHandler(Deps{Tasks: task.NewService(memory.New()), Log: logger})

	post := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(validBody))
	if rec := serve(h, post); rec.Code != http.StatusCreated {
		t.Fatalf("POST: status %d, body %s", rec.Code, rec.Body)
	}
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"a"`) {
		t.Errorf("GET: status %d, body %s", rec.Code, rec.Body)
	}
}
