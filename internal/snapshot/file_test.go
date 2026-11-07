package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lemotw/task-api/internal/task"
)

func newTestFile(t *testing.T) (*File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	return NewFile(path), path
}

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
}

// makeReadOnly removes write permission from dir until the test ends.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	skipIfRoot(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestFile_PathReturnsLocation(t *testing.T) {
	f, path := newTestFile(t)
	if got := f.Path(); got != path {
		t.Errorf("Path = %q, want %q", got, path)
	}
}

func TestFile_LoadReadsHandWrittenSnapshot(t *testing.T) {
	f, path := newTestFile(t)
	writeRaw(t, path, `{"version":1,"next_id":4,"tasks":[
		{"id":1,"name":"buy milk","status":0},
		{"id":3,"name":"  keep spaces  ","status":1}
	]}`)

	got, found, err := f.Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	want := task.State{NextID: 4, Tasks: []task.Task{
		{ID: 1, Name: "buy milk", Status: 0},
		{ID: 3, Name: "  keep spaces  ", Status: 1},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state = %+v, want %+v", got, want)
	}
}

func TestFile_LoadAcceptsEmptyTaskList(t *testing.T) {
	f, path := newTestFile(t)
	writeRaw(t, path, `{"version":1,"next_id":7,"tasks":[]}`)

	got, found, err := f.Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.NextID != 7 || len(got.Tasks) != 0 {
		t.Errorf("state = %+v", got)
	}
}

func TestFile_LoadMissingFileIsNotAnError(t *testing.T) {
	f, _ := newTestFile(t)
	_, found, err := f.Load()
	if err != nil || found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
}

func TestFile_SaveWritesDocumentedFormat(t *testing.T) {
	f, path := newTestFile(t)
	err := f.Save(task.State{NextID: 4, Tasks: []task.Task{{ID: 1, Name: "buy milk", Status: 1}}})
	if err != nil {
		t.Fatalf("Save = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	want := map[string]any{
		"version": float64(1),
		"next_id": float64(4),
		"tasks": []any{
			map[string]any{"id": float64(1), "name": "buy milk", "status": float64(1)},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("file = %v, want %v", got, want)
	}
}

func TestFile_SaveWritesEmptyListWithoutTasks(t *testing.T) {
	f, path := newTestFile(t)
	if err := f.Save(task.State{NextID: 1}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tasks json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Tasks) != "[]" {
		t.Errorf("tasks = %s, want []", got.Tasks)
	}
	if _, found, err := f.Load(); err != nil || !found {
		t.Errorf("Load: found=%v err=%v", found, err)
	}
}

func TestFile_RoundTripKeepsNextIDAndName(t *testing.T) {
	f, _ := newTestFile(t)
	// NextID is above every task: the highest id was deleted.
	want := task.State{NextID: 9, Tasks: []task.Task{
		{ID: 2, Name: "  任務 \"quoted\"\n", Status: 1},
	}}
	if err := f.Save(want); err != nil {
		t.Fatalf("Save = %v", err)
	}
	got, found, err := f.Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state = %+v, want %+v", got, want)
	}
}

func TestFile_SecondSaveReplacesContentWithoutLeftovers(t *testing.T) {
	f, path := newTestFile(t)
	first := task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "first"}}}
	second := task.State{NextID: 3, Tasks: []task.Task{{ID: 2, Name: "second"}}}
	for _, state := range []task.State{first, second} {
		if err := f.Save(state); err != nil {
			t.Fatalf("Save = %v", err)
		}
	}

	got, _, err := f.Load()
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if !reflect.DeepEqual(got, second) {
		t.Errorf("state = %+v", got)
	}
	want := []string{filepath.Base(path)}
	if names := dirEntries(t, filepath.Dir(path)); !reflect.DeepEqual(names, want) {
		t.Errorf("directory has %v", names)
	}
}

func TestFile_SaveUsesOwnerOnlyPermissions(t *testing.T) {
	f, path := newTestFile(t)
	if err := f.Save(task.State{NextID: 1}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o", got)
	}
}

func TestFile_CheckWritableLeavesNothingBehind(t *testing.T) {
	f, path := newTestFile(t)
	if err := f.CheckWritable(); err != nil {
		t.Fatalf("CheckWritable = %v", err)
	}
	if names := dirEntries(t, filepath.Dir(path)); len(names) != 0 {
		t.Errorf("directory has %v", names)
	}
}

func TestFile_LoadRejectsCorruptSnapshotsWithoutChangingThem(t *testing.T) {
	task1 := `{"id":1,"name":"a","status":0}`
	cases := map[string]string{
		"empty file":            ``,
		"not JSON":              `not json`,
		"truncated JSON":        `{"version":1,"next_id":2,"tasks":[`,
		"root is an array":      `[]`,
		"root is null":          `null`,
		"second JSON document":  `{"version":1,"next_id":1,"tasks":[]}{}`,
		"missing version":       `{"next_id":1,"tasks":[]}`,
		"missing next_id":       `{"version":1,"tasks":[]}`,
		"missing tasks":         `{"version":1,"next_id":1}`,
		"null tasks":            `{"version":1,"next_id":1,"tasks":null}`,
		"unsupported version":   `{"version":2,"next_id":1,"tasks":[]}`,
		"version wrong type":    `{"version":"1","next_id":1,"tasks":[]}`,
		"next_id wrong type":    `{"version":1,"next_id":"1","tasks":[]}`,
		"next_id fraction":      `{"version":1,"next_id":1.5,"tasks":[]}`,
		"tasks wrong type":      `{"version":1,"next_id":1,"tasks":{}}`,
		"next_id zero":          `{"version":1,"next_id":0,"tasks":[]}`,
		"next_id negative":      `{"version":1,"next_id":-3,"tasks":[]}`,
		"next_id equals max id": `{"version":1,"next_id":1,"tasks":[` + task1 + `]}`,
		"next_id below max id":  `{"version":1,"next_id":2,"tasks":[{"id":5,"name":"a","status":0}]}`,
		"duplicate id":          `{"version":1,"next_id":2,"tasks":[` + task1 + `,` + task1 + `]}`,
		"task id zero":          `{"version":1,"next_id":2,"tasks":[{"id":0,"name":"a","status":0}]}`,
		"task id negative":      `{"version":1,"next_id":2,"tasks":[{"id":-1,"name":"a","status":0}]}`,
		"task id wrong type":    `{"version":1,"next_id":2,"tasks":[{"id":"1","name":"a","status":0}]}`,
		"task missing id":       `{"version":1,"next_id":2,"tasks":[{"name":"a","status":0}]}`,
		"task missing name":     `{"version":1,"next_id":2,"tasks":[{"id":1,"status":0}]}`,
		"task missing status":   `{"version":1,"next_id":2,"tasks":[{"id":1,"name":"a"}]}`,
		"task id null":          `{"version":1,"next_id":2,"tasks":[{"id":null,"name":"a","status":0}]}`,
		"task name null":        `{"version":1,"next_id":2,"tasks":[{"id":1,"name":null,"status":0}]}`,
		"task status null":      `{"version":1,"next_id":2,"tasks":[{"id":1,"name":"a","status":null}]}`,
		"blank name":            `{"version":1,"next_id":2,"tasks":[{"id":1,"name":"  ","status":0}]}`,
		"name too long":         `{"version":1,"next_id":2,"tasks":[{"id":1,"name":"` + strings.Repeat("任", 256) + `","status":0}]}`,
		"invalid status":        `{"version":1,"next_id":2,"tasks":[{"id":1,"name":"a","status":2}]}`,
	}
	for label, content := range cases {
		t.Run(label, func(t *testing.T) {
			f, path := newTestFile(t)
			writeRaw(t, path, content)

			_, found, err := f.Load()
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Load: found=%v err=%v, want ErrCorrupt", found, err)
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(raw) != content {
				t.Errorf("file changed to %q", raw)
			}
		})
	}
}

func TestFile_LoadReportsReadFailureAsNotCorrupt(t *testing.T) {
	t.Run("path is a directory", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := NewFile(dir).Load()
		if err == nil || errors.Is(err, ErrCorrupt) {
			t.Fatalf("Load = %v, want a read error", err)
		}
	})
	t.Run("no read permission", func(t *testing.T) {
		skipIfRoot(t)
		f, path := newTestFile(t)
		writeRaw(t, path, `{"version":1,"next_id":1,"tasks":[]}`)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		_, _, err := f.Load()
		if err == nil || errors.Is(err, ErrCorrupt) {
			t.Fatalf("Load = %v, want a read error", err)
		}
	})
}

func TestFile_SaveToMissingDirectoryFails(t *testing.T) {
	f := NewFile(filepath.Join(t.TempDir(), "missing", "tasks.json"))
	if err := f.Save(task.State{NextID: 1}); err == nil {
		t.Fatal("Save returned no error")
	}
}

func TestFile_SaveToReadOnlyDirectoryKeepsOldFile(t *testing.T) {
	f, path := newTestFile(t)
	old := task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "old"}}}
	if err := f.Save(old); err != nil {
		t.Fatalf("Save = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	makeReadOnly(t, filepath.Dir(path))
	if err := f.Save(task.State{NextID: 1}); err == nil {
		t.Fatal("Save returned no error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("file changed to %q", after)
	}
}

// Writing in place would keep the same file and could leave it half written. A
// replaced file has a new identity, which proves Save goes through rename (PER-11).
func TestFile_SaveReplacesFileInsteadOfWritingInPlace(t *testing.T) {
	f, path := newTestFile(t)
	if err := f.Save(task.State{NextID: 1}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Save(task.State{NextID: 2}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("Save wrote into the existing file")
	}
}

// A leftover path next to the snapshot, here a symlink back to it, must not be
// written through: Save creates its own temporary file (PER-11).
func TestFile_SaveIgnoresExistingPathsNextToTheSnapshot(t *testing.T) {
	f, path := newTestFile(t)
	if err := f.Save(task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "old"}}}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	if err := os.Symlink(path, path+".tmp"); err != nil {
		t.Fatal(err)
	}

	want := task.State{NextID: 3, Tasks: []task.Task{{ID: 2, Name: "new"}}}
	if err := f.Save(want); err != nil {
		t.Fatalf("Save = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("snapshot is %v, want a regular file", info.Mode())
	}
	got, _, err := f.Load()
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state = %+v, want %+v", got, want)
	}
}

func TestFile_RenameFailureLeavesNoTemporaryFile(t *testing.T) {
	f, path := newTestFile(t)
	if err := os.Mkdir(path, 0o700); err != nil { // rename onto a directory fails
		t.Fatal(err)
	}
	if err := f.Save(task.State{NextID: 1}); err == nil {
		t.Fatal("Save returned no error")
	}
	want := []string{filepath.Base(path)}
	if names := dirEntries(t, filepath.Dir(path)); !reflect.DeepEqual(names, want) {
		t.Errorf("directory has %v", names)
	}
}

func TestFile_CheckWritableFails(t *testing.T) {
	t.Run("read-only directory", func(t *testing.T) {
		f, path := newTestFile(t)
		makeReadOnly(t, filepath.Dir(path))
		if err := f.CheckWritable(); err == nil {
			t.Fatal("CheckWritable returned no error")
		}
	})
	t.Run("missing directory", func(t *testing.T) {
		f := NewFile(filepath.Join(t.TempDir(), "missing", "tasks.json"))
		if err := f.CheckWritable(); err == nil {
			t.Fatal("CheckWritable returned no error")
		}
	})
}
