package task

// Repository stores tasks (ARC-5). Every method MUST be safe for concurrent
// use, and Update and Delete MUST check existence and modify within one
// critical section (ARC-7). Implementations do not validate input.
type Repository interface {
	// List returns all tasks ordered by ascending id, never nil.
	List() []Task
	// Create assigns the next id and stores a new task.
	Create(name string, status int) Task
	// Update replaces name and status, or returns ErrNotFound.
	Update(id int, name string, status int) (Task, error)
	// Delete removes a task, or returns ErrNotFound.
	Delete(id int) error
}
