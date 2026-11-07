// Package memory provides the in-memory task.Repository (ARC-6).
package memory

import (
	"sort"
	"sync"

	"github.com/lemotw/task-api/internal/task"
)

var _ task.Repository = (*Repository)(nil)

// Repository is an in-memory task store.
type Repository struct {
	mu       sync.RWMutex
	tasks    map[int]task.Task
	nextID   int
	revision uint64 // incremented on every state change
}

// New returns an empty Repository.
func New() *Repository {
	return &Repository{tasks: make(map[int]task.Task), nextID: 1}
}

// List implements task.Repository.
func (r *Repository) List() []task.Task {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sortedTasks()
}

// Create implements task.Repository.
func (r *Repository) Create(name string, status int) task.Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := task.Task{ID: r.nextID, Name: name, Status: status}
	r.tasks[t.ID] = t
	r.nextID++
	r.revision++
	return t
}

// Update implements task.Repository.
func (r *Repository) Update(id int, name string, status int) (task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[id]; !ok {
		return task.Task{}, task.ErrNotFound
	}
	t := task.Task{ID: id, Name: name, Status: status}
	r.tasks[id] = t
	r.revision++
	return t, nil
}

// Delete implements task.Repository.
func (r *Repository) Delete(id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[id]; !ok {
		return task.ErrNotFound
	}
	delete(r.tasks, id)
	r.revision++
	return nil
}

// Export returns the state and its revision from one critical section (PER-4).
// Tasks are ordered by ascending id, so equal states always export identically.
func (r *Repository) Export() (task.State, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return task.State{NextID: r.nextID, Tasks: r.sortedTasks()}, r.revision
}

// Import replaces the state. It is called before serving requests (PER-5).
func (r *Repository) Import(state task.State) {
	tasks := make(map[int]task.Task, len(state.Tasks))
	for _, t := range state.Tasks {
		tasks[t.ID] = t
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = tasks
	r.nextID = state.NextID
	r.revision++
}

// sortedTasks returns a copy of the tasks by ascending id. The caller holds mu.
func (r *Repository) sortedTasks() []task.Task {
	out := make([]task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
