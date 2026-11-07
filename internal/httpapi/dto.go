package httpapi

import "github.com/lemotw/task-api/internal/task"

// taskRequest is the POST and PUT body. Pointers separate a missing field from
// a zero value (ARC-3, API-5).
type taskRequest struct {
	Name   *string `json:"name"`
	Status *int    `json:"status"`
}

// taskResponse is the JSON representation of a task.
type taskResponse struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

// errorResponse is the error body (API-16).
type errorResponse struct {
	Error string `json:"error"`
}

func toResponse(t task.Task) taskResponse {
	return taskResponse{ID: t.ID, Name: t.Name, Status: t.Status}
}
