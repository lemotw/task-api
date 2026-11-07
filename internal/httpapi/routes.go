package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// routes registers every API path and its handler. To add an endpoint, add it
// here. See docs/spec/core/02-api.md.
func (h *handler) routes(r chi.Router) {
	r.Get("/tasks", h.listTasks)          // API-10
	r.Post("/tasks", h.createTask)        // API-11
	r.Put("/tasks/{id}", h.updateTask)    // API-12
	r.Delete("/tasks/{id}", h.deleteTask) // API-13
}

// apiMethods are the methods the API uses, in the order the Allow header lists
// them (API-14).
var apiMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

// newRouter builds the chi router: the API routes, then one route per web UI
// page, with JSON responses for unknown paths and methods (API-16). chi itself
// checks the path, then the method; handlers check {id} next (API-7).
func (h *handler) newRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(h.accessLog)
	h.routes(r)
	for path, page := range h.deps.Pages {
		r.Handle(path, page) // every method goes to the page (UI-2)
	}
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(h.methodNotAllowed)
	return r
}

// methodNotAllowed answers 405 with the Allow header (API-14). chi does not pass
// the allowed methods to a custom handler, so they are found by matching the
// path against each API method.
func (h *handler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	var allowed []string
	for _, method := range apiMethods {
		if pattern := h.router.Find(chi.NewRouteContext(), method, r.URL.Path); pattern != "" {
			allowed = append(allowed, method)
			setRoute(r, pattern)
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
