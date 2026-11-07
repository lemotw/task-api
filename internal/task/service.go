package task

// Service applies the domain rules and delegates storage to a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service {
	panic("not implemented")
}

// List returns all tasks ordered by ascending id.
func (s *Service) List() []Task {
	panic("not implemented")
}

// Create validates the values and stores a new task.
// It returns *ValidationError for invalid values.
func (s *Service) Create(name string, status int) (Task, error) {
	panic("not implemented")
}

// Update validates the values and replaces an existing task.
// It returns *ValidationError or ErrNotFound.
func (s *Service) Update(id int, name string, status int) (Task, error) {
	panic("not implemented")
}

// Delete removes a task. It returns ErrNotFound if the id does not exist.
func (s *Service) Delete(id int) error {
	panic("not implemented")
}
