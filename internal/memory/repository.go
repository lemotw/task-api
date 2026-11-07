// Package memory provides the in-memory task.Repository (ARC-6).
package memory

import (
	"sync"

	"github.com/lemo/task-api/internal/task"
)

var _ task.Repository = (*Repository)(nil)

// Repository is an in-memory task store.
type Repository struct {
	mu       sync.RWMutex
	tasks    map[int]task.Task
	nextID   int
	revision uint64 // incremented on every successful mutation
}

// New returns an empty Repository.
func New() *Repository {
	panic("not implemented")
}

// List implements task.Repository.
func (r *Repository) List() []task.Task {
	panic("not implemented")
}

// Create implements task.Repository.
func (r *Repository) Create(name string, status int) task.Task {
	panic("not implemented")
}

// Update implements task.Repository.
func (r *Repository) Update(id int, name string, status int) (task.Task, error) {
	panic("not implemented")
}

// Delete implements task.Repository.
func (r *Repository) Delete(id int) error {
	panic("not implemented")
}

// Export returns the state and its revision from one critical section (PER-4).
func (r *Repository) Export() (task.State, uint64) {
	panic("not implemented")
}

// Import replaces the state. It is called before serving requests (PER-5).
func (r *Repository) Import(state task.State) {
	panic("not implemented")
}
