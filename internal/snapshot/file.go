package snapshot

import "github.com/lemo/task-api/internal/task"

var _ Storage = (*File)(nil)

// File stores a snapshot in the JSON format defined by PER-3.
type File struct {
	path string
}

// NewFile returns a File for path.
func NewFile(path string) *File {
	panic("not implemented")
}

// Load implements Storage.
func (f *File) Load() (task.State, bool, error) {
	panic("not implemented")
}

// Save implements Storage.
func (f *File) Save(state task.State) error {
	panic("not implemented")
}

// CheckWritable implements Storage.
func (f *File) CheckWritable() error {
	panic("not implemented")
}

// Path implements Storage.
func (f *File) Path() string {
	panic("not implemented")
}
