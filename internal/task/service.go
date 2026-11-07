package task

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Validate checks name and status against DOM-4, DOM-5 and DOM-8. It returns
// *ValidationError for invalid values. Service and snapshot share these rules.
func Validate(name string, status int) error {
	if strings.TrimSpace(name) == "" {
		return &ValidationError{Field: "name", Message: "name must contain a non-whitespace character"}
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("name must not exceed %d characters", MaxNameLength),
		}
	}
	if status != StatusIncomplete && status != StatusComplete {
		return &ValidationError{Field: "status", Message: "status must be 0 or 1"}
	}
	return nil
}

// Service applies the domain rules and delegates storage to a Repository.
type Service struct {
	repo Repository
}

// NewService returns a Service backed by repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns all tasks ordered by ascending id.
func (s *Service) List() []Task {
	return s.repo.List()
}

// Create validates the values and stores a new task.
// It returns *ValidationError for invalid values.
func (s *Service) Create(name string, status int) (Task, error) {
	if err := Validate(name, status); err != nil {
		return Task{}, err
	}
	return s.repo.Create(name, status), nil
}

// Update validates the values and replaces an existing task.
// It returns *ValidationError or ErrNotFound.
func (s *Service) Update(id int, name string, status int) (Task, error) {
	if err := Validate(name, status); err != nil {
		return Task{}, err
	}
	return s.repo.Update(id, name, status)
}

// Delete removes a task. It returns ErrNotFound if the id does not exist.
func (s *Service) Delete(id int) error {
	return s.repo.Delete(id)
}
