package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lemotw/task-api/internal/task"
)

const (
	formatVersion = 1
	filePerm      = 0o600
)

var _ Storage = (*File)(nil)

// fileFormat is the snapshot on disk (PER-3). Pointers separate a missing or
// null field from a zero value.
type fileFormat struct {
	Version *int        `json:"version"`
	NextID  *int        `json:"next_id"`
	Tasks   *[]fileTask `json:"tasks"`
}

type fileTask struct {
	ID     *int    `json:"id"`
	Name   *string `json:"name"`
	Status *int    `json:"status"`
}

// File stores a snapshot in the JSON format defined by PER-3.
type File struct {
	path string
}

// NewFile returns a File for path.
func NewFile(path string) *File {
	return &File{path: path}
}

// Load implements Storage.
func (f *File) Load() (task.State, bool, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return task.State{}, false, nil
	}
	if err != nil {
		return task.State{}, false, fmt.Errorf("read snapshot: %w", err)
	}
	state, err := decodeState(data)
	if err != nil {
		return task.State{}, false, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return state, true, nil
}

// Save implements Storage. It writes a new temporary file in the same directory
// and renames it over the target, so the old snapshot stays whole on failure
// (PER-11). The temporary file is always newly created, so nothing already in
// the directory, such as a symlink, is written through. A crash can leave a
// temporary file behind; it is never read.
func (f *File) Save(state task.State) error {
	data, err := encodeState(state)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}

	tmp, err := writeTemp(filepath.Dir(f.path), filepath.Base(f.path)+".tmp-*", data)
	if err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace snapshot: %w", err)
	}
	syncDir(filepath.Dir(f.path))
	return nil
}

// CheckWritable implements Storage.
func (f *File) CheckWritable() error {
	probe, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".check-*")
	if err != nil {
		return fmt.Errorf("check snapshot directory: %w", err)
	}
	_ = probe.Close()
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("check snapshot directory: %w", err)
	}
	return nil
}

// Path implements Storage.
func (f *File) Path() string {
	return f.path
}

// writeTemp creates a new file in dir, writes data, syncs it and returns its
// path. On failure it removes the file.
func writeTemp(dir, pattern string, data []byte) (path string, err error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	if err := file.Chmod(filePerm); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil
}

// syncDir makes the rename durable where possible (PER-12). The new snapshot is
// already in place, and not every platform can sync a directory, so a failure
// is not reported.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}

func encodeState(state task.State) ([]byte, error) {
	tasks := make([]fileTask, 0, len(state.Tasks))
	for i := range state.Tasks {
		t := &state.Tasks[i]
		tasks = append(tasks, fileTask{ID: &t.ID, Name: &t.Name, Status: &t.Status})
	}
	version := formatVersion
	data, err := json.MarshalIndent(fileFormat{
		Version: &version,
		NextID:  &state.NextID,
		Tasks:   &tasks,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// decodeState parses and validates a snapshot against PER-7.
func decodeState(data []byte) (task.State, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var file fileFormat
	if err := dec.Decode(&file); err != nil {
		return task.State{}, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return task.State{}, errors.New("unexpected data after the snapshot")
	}

	if file.Version == nil || file.NextID == nil || file.Tasks == nil {
		return task.State{}, errors.New("version, next_id and tasks are required")
	}
	if *file.Version != formatVersion {
		return task.State{}, fmt.Errorf("unsupported version %d", *file.Version)
	}
	if *file.NextID <= 0 {
		return task.State{}, fmt.Errorf("next_id %d is not positive", *file.NextID)
	}

	state := task.State{NextID: *file.NextID, Tasks: make([]task.Task, 0, len(*file.Tasks))}
	seen := make(map[int]bool, len(*file.Tasks))
	for i, t := range *file.Tasks {
		if t.ID == nil || t.Name == nil || t.Status == nil {
			return task.State{}, fmt.Errorf("task %d: id, name and status are required", i)
		}
		if *t.ID <= 0 {
			return task.State{}, fmt.Errorf("task %d: id %d is not positive", i, *t.ID)
		}
		if seen[*t.ID] {
			return task.State{}, fmt.Errorf("task %d: duplicate id %d", i, *t.ID)
		}
		seen[*t.ID] = true
		if *t.ID >= *file.NextID {
			return task.State{}, fmt.Errorf("task %d: id %d is not below next_id %d", i, *t.ID, *file.NextID)
		}
		if err := task.Validate(*t.Name, *t.Status); err != nil {
			return task.State{}, fmt.Errorf("task %d: %w", i, err)
		}
		state.Tasks = append(state.Tasks, task.Task{ID: *t.ID, Name: *t.Name, Status: *t.Status})
	}
	return state, nil
}
