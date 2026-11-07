// Package httpapi is the HTTP layer. See docs/spec/core/02-api.md.
package httpapi

import (
	"net/http"

	"github.com/lemo/task-api/internal/logging"
	"github.com/lemo/task-api/internal/task"
)

// MaxBodyBytes is the request body limit (API-6).
const MaxBodyBytes = 1 << 20

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
	// UI serves GET / when set. It is nil when the web UI is disabled.
	UI http.Handler
}

// NewHandler returns the root handler, including access logging (LOG-11).
func NewHandler(deps Deps) http.Handler {
	panic("not implemented")
}
