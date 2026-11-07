// Package httpapi is the HTTP layer. See docs/spec/core/02-api.md.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lemotw/task-api/internal/logging"
	"github.com/lemotw/task-api/internal/task"
)

// MaxBodyBytes is the request body limit (API-6).
const MaxBodyBytes = 1 << 20

const (
	routeUnmatched     = "unmatched"
	eventInternalError = "http.internal_error"
)

// idPattern is the only accepted form of {id} (API-3).
var idPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// TaskService is the behaviour the HTTP layer needs from the service layer.
type TaskService interface {
	List() []task.Task
	Create(name string, status int) (task.Task, error)
	Update(id int, name string, status int) (task.Task, error)
	Delete(id int) error
}

// Deps are the dependencies of the HTTP handler.
type Deps struct {
	Tasks TaskService
	Log   logging.Logger
	// Pages serves the web UI, keyed by exact path (UI-7). It is nil when the
	// web UI is disabled.
	Pages map[string]http.Handler
}

// handler serves the API with the routes registered in routes.go.
type handler struct {
	deps   Deps
	router *chi.Mux
}

// NewHandler returns the root handler, including access logging (LOG-11).
func NewHandler(deps Deps) http.Handler {
	h := &handler{deps: deps}
	h.router = h.newRouter()
	return h.router
}

type routeKey struct{}

// accessLog writes one access event per request after the response (LOG-11).
// The route is the matched pattern, or "unmatched" for unknown paths.
func (h *handler) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, head: r.Method == http.MethodHead}
		route := new(string)
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), routeKey{}, route)))
		if *route == "" {
			*route = chi.RouteContext(r.Context()).RoutePattern()
		}
		if *route == "" {
			*route = routeUnmatched
		}
		h.deps.Log.Access(logging.AccessEvent{
			Method:     r.Method,
			Route:      *route,
			Path:       r.URL.Path,
			Status:     rec.statusCode(),
			DurationMS: time.Since(start).Milliseconds(),
			Bytes:      rec.bytes,
		})
	})
}

// setRoute records the route for the access log when chi has not matched one,
// as for 405 responses.
func setRoute(r *http.Request, pattern string) {
	if route, ok := r.Context().Value(routeKey{}).(*string); ok && *route == "" {
		*route = pattern
	}
}

// taskID reads {id} (API-3). On failure it writes 400 and returns false.
func taskID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, ok := parseID(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid task id")
	}
	return id, ok
}

func routePattern(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePattern()
}

func (h *handler) listTasks(w http.ResponseWriter, _ *http.Request) {
	tasks := h.deps.Tasks.List()
	out := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toResponse(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) createTask(w http.ResponseWriter, r *http.Request) {
	req, ok := readTaskRequest(w, r)
	if !ok {
		return
	}
	created, err := h.deps.Tasks.Create(*req.Name, *req.Status)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/tasks/"+strconv.Itoa(created.ID))
	writeJSON(w, http.StatusCreated, toResponse(created))
}

func (h *handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	req, ok := readTaskRequest(w, r)
	if !ok {
		return
	}
	updated, err := h.deps.Tasks.Update(id, *req.Name, *req.Status)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(updated))
}

func (h *handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	if err := h.deps.Tasks.Delete(id); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseID(segment string) (int, bool) {
	if !idPattern.MatchString(segment) {
		return 0, false
	}
	id, err := strconv.Atoi(segment)
	return id, err == nil
}

// readTaskRequest reads the whole body before parsing, so an oversized body is
// reported as 413 even if it starts with invalid JSON (API-7). On failure it
// writes the response and returns false.
func readTaskRequest(w http.ResponseWriter, r *http.Request) (taskRequest, bool) {
	var req taskRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "cannot read request body")
		}
		return req, false
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return req, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body: unexpected data after the JSON value")
		return req, false
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "name is required")
		return req, false
	}
	if req.Status == nil {
		writeError(w, http.StatusBadRequest, "status is required")
		return req, false
	}
	return req, true
}

// writeServiceError maps domain errors to status codes (ARC-4). Anything else
// is logged and hidden from the client (API-18).
func (h *handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *task.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message)
	case errors.Is(err, task.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	default:
		h.deps.Log.Error(eventInternalError,
			logging.Field{Key: "route", Value: routePattern(r)},
			logging.Field{Key: "path", Value: r.URL.Path},
			logging.Field{Key: "error", Value: err.Error()},
		)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		buf.Reset()
		status = http.StatusInternalServerError
		buf.WriteString(`{"error":"internal server error"}` + "\n")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes()) // the client is gone; nothing more can be done
}

// statusRecorder captures the status and size for the access log. The size is
// what reaches the client, so it stays 0 for HEAD, whose body is never sent.
type statusRecorder struct {
	http.ResponseWriter
	head   bool
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	n, err := r.ResponseWriter.Write(data)
	if !r.head {
		r.bytes += int64(n)
	}
	return n, err
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
