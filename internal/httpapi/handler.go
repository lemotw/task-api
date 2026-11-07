// Package httpapi is the HTTP layer. See docs/spec/core/02-api.md.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lemo/task-api/internal/logging"
	"github.com/lemo/task-api/internal/task"
)

// MaxBodyBytes is the request body limit (API-6).
const MaxBodyBytes = 1 << 20

const (
	routeTasks     = "/tasks"
	routeTask      = "/tasks/{id}"
	routeUnmatched = "unmatched"

	allowTasks = "GET, POST"
	allowTask  = "PUT, DELETE"

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

// handler routes requests in one place so the check order of API-7 is explicit.
// net/http's ServeMux is not used because it answers HEAD on GET routes and
// writes its own plain-text 404 and 405 responses.
type handler struct {
	deps Deps
}

// NewHandler returns the root handler, including access logging (LOG-11).
func NewHandler(deps Deps) http.Handler {
	return &handler{deps: deps}
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w}
	route := h.route(rec, r)
	h.deps.Log.Access(logging.AccessEvent{
		Method:     r.Method,
		Route:      route,
		Path:       r.URL.Path,
		Status:     rec.statusCode(),
		DurationMS: time.Since(start).Milliseconds(),
		Bytes:      rec.bytes,
	})
}

// route serves the request and returns the route name for the access log.
func (h *handler) route(w *statusRecorder, r *http.Request) string {
	path := r.URL.Path
	switch {
	case path == routeTasks:
		h.serveTasks(w, r)
		return routeTasks
	case strings.HasPrefix(path, routeTasks+"/"):
		segment := strings.TrimPrefix(path, routeTasks+"/")
		if segment == "" || strings.Contains(segment, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return routeUnmatched
		}
		h.serveTask(w, r, segment)
		return routeTask
	case h.deps.Pages[path] != nil:
		h.deps.Pages[path].ServeHTTP(w, r)
		return path
	default:
		writeError(w, http.StatusNotFound, "not found")
		return routeUnmatched
	}
}

func (h *handler) serveTasks(w *statusRecorder, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := h.deps.Tasks.List()
		out := make([]taskResponse, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, toResponse(t))
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		req, ok := readTaskRequest(w, r)
		if !ok {
			return
		}
		created, err := h.deps.Tasks.Create(*req.Name, *req.Status)
		if err != nil {
			h.writeServiceError(w, r, routeTasks, err)
			return
		}
		w.Header().Set("Location", routeTasks+"/"+strconv.Itoa(created.ID))
		writeJSON(w, http.StatusCreated, toResponse(created))
	default:
		writeMethodNotAllowed(w, allowTasks)
	}
}

func (h *handler) serveTask(w *statusRecorder, r *http.Request, segment string) {
	id, ok := parseID(segment)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := readTaskRequest(w, r)
		if !ok {
			return
		}
		updated, err := h.deps.Tasks.Update(id, *req.Name, *req.Status)
		if err != nil {
			h.writeServiceError(w, r, routeTask, err)
			return
		}
		writeJSON(w, http.StatusOK, toResponse(updated))
	case http.MethodDelete:
		if err := h.deps.Tasks.Delete(id); err != nil {
			h.writeServiceError(w, r, routeTask, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, allowTask)
	}
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
func readTaskRequest(w *statusRecorder, r *http.Request) (taskRequest, bool) {
	var req taskRequest
	body, err := io.ReadAll(http.MaxBytesReader(w.ResponseWriter, r.Body, MaxBodyBytes))
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
func (h *handler) writeServiceError(w *statusRecorder, r *http.Request, route string, err error) {
	var invalid *task.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message)
	case errors.Is(err, task.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	default:
		h.deps.Log.Error(eventInternalError,
			logging.Field{Key: "route", Value: route},
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

// statusRecorder captures the status and size for the access log.
type statusRecorder struct {
	http.ResponseWriter
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
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
