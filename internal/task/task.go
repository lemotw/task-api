// Package task holds the domain model, domain errors, Service and the
// Repository interface. See docs/spec/core/01-domain.md.
package task

import "errors"

// Status values (DOM-8).
const (
	StatusIncomplete = 0
	StatusComplete   = 1
)

// MaxNameLength is the maximum name length in Unicode code points (DOM-5).
const MaxNameLength = 255

// Task is the domain resource (DOM-1). It carries no transport or storage tags.
type Task struct {
	ID     int
	Name   string
	Status int
}

// State is the complete repository state at one point in time.
// NextID is kept so ids are never reused (DOM-3).
type State struct {
	NextID int
	Tasks  []Task
}

// ErrNotFound reports that the target id does not exist (DOM-13).
var ErrNotFound = errors.New("task not found")

// ValidationError reports input that violates DOM-4, DOM-5 or DOM-8 (DOM-12).
type ValidationError struct {
	Field   string
	Message string
}

// Error implements error.
func (e *ValidationError) Error() string {
	panic("not implemented")
}
