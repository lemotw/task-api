package snapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lemo/task-api/internal/logging"
	"github.com/lemo/task-api/internal/logging/logtest"
	"github.com/lemo/task-api/internal/task"
)

const (
	testInterval = 5 * time.Second
	testPath     = "/data/tasks.json"
)

var errBoom = errors.New("boom")

type fakeSource struct {
	mu      sync.Mutex
	state   task.State
	rev     uint64
	imports []task.State
}

func newFakeSource() *fakeSource {
	return &fakeSource{state: task.State{NextID: 1}}
}

func (s *fakeSource) Export() (task.State, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return task.State{
		NextID: s.state.NextID,
		Tasks:  append([]task.Task(nil), s.state.Tasks...),
	}, s.rev
}

func (s *fakeSource) Import(state task.State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.imports = append(s.imports, state)
}

func (s *fakeSource) importCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.imports)
}

// change simulates a successful mutation of the repository.
func (s *fakeSource) change(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Tasks = append(s.state.Tasks, task.Task{ID: s.state.NextID, Name: name})
	s.state.NextID++
	s.rev++
}

type fakeStorage struct {
	mu       sync.Mutex
	loaded   task.State
	found    bool
	loadErr  error
	checkErr error
	saveFn   func(task.State) error // runs without holding mu
	attempts int
	active   int
	maxAct   int
	saved    []task.State
}

func (s *fakeStorage) Load() (task.State, bool, error) {
	return s.loaded, s.found, s.loadErr
}

func (s *fakeStorage) Save(state task.State) error {
	s.mu.Lock()
	s.attempts++
	s.active++
	s.maxAct = max(s.maxAct, s.active)
	fn := s.saveFn
	s.mu.Unlock()

	var err error
	if fn != nil {
		err = fn(state)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	if err == nil {
		s.saved = append(s.saved, state)
	}
	return err
}

func (s *fakeStorage) CheckWritable() error { return s.checkErr }

func (s *fakeStorage) Path() string { return testPath }

func (s *fakeStorage) setSaveFn(fn func(task.State) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveFn = fn
}

func (s *fakeStorage) attemptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts
}

func (s *fakeStorage) savedStates() []task.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]task.State(nil), s.saved...)
}

type fixture struct {
	source  *fakeSource
	storage *fakeStorage
	log     *logtest.Recorder
	syncer  *Syncer
}

func newFixture(interval time.Duration) *fixture {
	f := &fixture{source: newFakeSource(), storage: &fakeStorage{}, log: &logtest.Recorder{}}
	f.syncer = NewSyncer(f.source, f.storage, interval, f.log)
	return f
}

// newLoadedFixture returns a fixture that has completed the startup load.
func newLoadedFixture(t *testing.T, interval time.Duration) *fixture {
	t.Helper()
	f := newFixture(interval)
	if err := f.syncer.Load(); err != nil {
		t.Fatalf("Load = %v", err)
	}
	return f
}

// startRun runs Syncer.Run and returns a function that stops it and waits.
func (f *fixture) startRun(t *testing.T) (stop func(), done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		f.syncer.Run(ctx)
	}()
	return func() {
		cancel()
		<-finished
	}, finished
}

// assertSaved compares every saved state, in order, with want.
func assertSaved(t *testing.T, f *fixture, want ...task.State) {
	t.Helper()
	got := f.storage.savedStates()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("saved states\n got: %+v\nwant: %+v", got, want)
	}
}

func assertFailureLogged(t *testing.T, log *logtest.Recorder, event string) {
	t.Helper()
	entries := log.Find(event)
	if len(entries) == 0 {
		t.Fatalf("no %s event", event)
	}
	e := entries[0]
	if e.Level != logging.LevelError {
		t.Errorf("level = %v", e.Level)
	}
	if got := fmt.Sprint(e.Fields["path"]); got != testPath {
		t.Errorf("path = %q", got)
	}
	if got := fmt.Sprint(e.Fields["error"]); !strings.Contains(got, "boom") {
		t.Errorf("error = %q", got)
	}
}

func TestLoad_ImportsFoundSnapshot(t *testing.T) {
	f := newFixture(testInterval)
	want := task.State{NextID: 4, Tasks: []task.Task{{ID: 1, Name: "a"}}}
	f.storage.loaded, f.storage.found = want, true

	if err := f.syncer.Load(); err != nil {
		t.Fatalf("Load = %v", err)
	}
	if !reflect.DeepEqual(f.source.imports, []task.State{want}) {
		t.Errorf("imports = %+v", f.source.imports)
	}
}

func TestLoad_MissingSnapshotStartsEmpty(t *testing.T) {
	f := newFixture(testInterval)
	if err := f.syncer.Load(); err != nil {
		t.Fatalf("Load = %v", err)
	}
	if n := f.source.importCount(); n != 0 {
		t.Errorf("Import called %d times", n)
	}
}

func TestLoad_CorruptSnapshotFailsWithoutImportOrWrite(t *testing.T) {
	f := newFixture(testInterval)
	f.storage.loadErr = fmt.Errorf("%w: boom", ErrCorrupt)

	if err := f.syncer.Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load = %v", err)
	}
	if n := f.source.importCount(); n != 0 {
		t.Errorf("Import called %d times", n)
	}
	if n := f.storage.attemptCount(); n != 0 {
		t.Errorf("Save called %d times", n)
	}
	assertFailureLogged(t, f.log, "snapshot.load_failed")
}

func TestLoad_ReadFailureFailsWithoutImport(t *testing.T) {
	f := newFixture(testInterval)
	f.storage.loadErr = errBoom

	if err := f.syncer.Load(); !errors.Is(err, errBoom) {
		t.Fatalf("Load = %v", err)
	}
	if n := f.source.importCount(); n != 0 {
		t.Errorf("Import called %d times", n)
	}
	assertFailureLogged(t, f.log, "snapshot.load_failed")
}

func TestLoad_UnwritableLocationFails(t *testing.T) {
	f := newFixture(testInterval)
	f.storage.checkErr = errBoom
	if err := f.syncer.Load(); !errors.Is(err, errBoom) {
		t.Fatalf("Load = %v", err)
	}
}

func TestFlush_AfterLoadWithoutChangesDoesNotWrite(t *testing.T) {
	f := newFixture(testInterval)
	f.storage.loaded = task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}}
	f.storage.found = true
	// A non-zero revision after the load: a Syncer that wrongly assumes a
	// baseline of 0 would see "unsaved changes" and write.
	f.source.rev = 7
	if err := f.syncer.Load(); err != nil {
		t.Fatalf("Load = %v", err)
	}
	if err := f.syncer.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}
	if n := f.storage.attemptCount(); n != 0 {
		t.Fatalf("Save called %d times without changes", n)
	}

	// The baseline is recorded, not ignored: a real change is still written.
	f.source.change("b")
	if err := f.syncer.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}
	assertSaved(t, f, task.State{NextID: 3, Tasks: []task.Task{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}})
}

func TestFlush_WritesPendingChangesOnce(t *testing.T) {
	f := newLoadedFixture(t, testInterval)
	f.source.change("a")
	for range 2 {
		if err := f.syncer.Flush(); err != nil {
			t.Fatalf("Flush = %v", err)
		}
	}
	assertSaved(t, f, task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}})
}

func TestFlush_WorksWhenPeriodicFlushIsDisabled(t *testing.T) {
	f := newLoadedFixture(t, 0)
	f.source.change("a")
	if err := f.syncer.Flush(); err != nil {
		t.Fatalf("Flush = %v", err)
	}
	assertSaved(t, f, task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}})
}

func TestFlush_SaveFailureIsReturnedAndLogged(t *testing.T) {
	f := newLoadedFixture(t, testInterval)
	f.storage.saveFn = func(task.State) error { return errBoom }
	f.source.change("a")

	if err := f.syncer.Flush(); !errors.Is(err, errBoom) {
		t.Fatalf("Flush = %v", err)
	}
	assertFailureLogged(t, f.log, "snapshot.flush_failed")
}

func TestRun_WritesChangeOnNextInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()
		assertSaved(t, f, task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}})
	})
}

func TestRun_DoesNotWriteWithoutChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		stop, _ := f.startRun(t)
		defer stop()

		time.Sleep(10*testInterval + time.Millisecond)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 0 {
			t.Errorf("Save called %d times", n)
		}
	})
}

func TestRun_WritesEachChangeOnlyOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(5*testInterval + time.Millisecond)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 1 {
			t.Errorf("Save called %d times", n)
		}
	})
}

func TestRun_ZeroIntervalDoesNotWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, 0)
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(time.Hour)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 0 {
			t.Errorf("Save called %d times", n)
		}
	})
}

func TestRun_CancelWaitsForWriteInProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		release := make(chan struct{})
		f.storage.setSaveFn(func(task.State) error {
			<-release
			return nil
		})
		stop, done := f.startRun(t)

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 1 {
			t.Fatalf("Save called %d times", n)
		}

		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			stop()
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Error("Run returned before the write finished")
		default:
		}

		close(release)
		<-stopped
	})
}

func TestRun_FailedWriteIsLoggedAndRetriedWithoutNewChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		f.storage.setSaveFn(func(task.State) error { return errBoom })
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 1 {
			t.Fatalf("Save called %d times after first interval", n)
		}
		assertFailureLogged(t, f.log, "snapshot.flush_failed")

		time.Sleep(testInterval)
		synctest.Wait()
		if n := f.storage.attemptCount(); n != 2 {
			t.Errorf("Save called %d times after retry interval", n)
		}
	})
}

func TestRun_SuccessfulRetryStopsWriting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		f.storage.setSaveFn(func(task.State) error { return errBoom })
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()

		f.storage.setSaveFn(nil)
		time.Sleep(testInterval)
		synctest.Wait()
		want := task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}}
		assertSaved(t, f, want)

		time.Sleep(3 * testInterval)
		synctest.Wait()
		assertSaved(t, f, want)
	})
}

func TestRun_ChangeDuringWriteIsWrittenNextInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		f.storage.setSaveFn(func(task.State) error {
			f.source.change("during write")
			f.storage.setSaveFn(nil)
			return nil
		})
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()

		time.Sleep(testInterval)
		synctest.Wait()
		// First write holds only A; the change made during it is not lost
		// and lands in a second write with the correct NextID.
		assertSaved(t, f,
			task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}},
			task.State{NextID: 3, Tasks: []task.Task{{ID: 1, Name: "a"}, {ID: 2, Name: "during write"}}},
		)
	})
}

func TestRun_SlowWritesNeverOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		f.storage.setSaveFn(func(task.State) error {
			f.source.change("more")
			time.Sleep(3 * testInterval)
			return nil
		})
		stop, _ := f.startRun(t)
		defer stop()

		f.source.change("a")
		time.Sleep(20 * testInterval)
		synctest.Wait()

		f.storage.mu.Lock()
		defer f.storage.mu.Unlock()
		if f.storage.attempts < 2 {
			t.Errorf("Save called %d times", f.storage.attempts)
		}
		if f.storage.maxAct != 1 {
			t.Errorf("%d concurrent saves", f.storage.maxAct)
		}
	})
}

func TestFlush_AfterCancelSavesChangeMadeDuringBlockedWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newLoadedFixture(t, testInterval)
		release := make(chan struct{})
		f.storage.setSaveFn(func(task.State) error {
			<-release
			return nil
		})
		stop, _ := f.startRun(t)

		f.source.change("a")
		time.Sleep(testInterval + time.Millisecond)
		synctest.Wait()

		f.source.change("b")
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			stop()
		}()
		synctest.Wait()
		close(release)
		<-stopped

		f.storage.setSaveFn(nil)
		if err := f.syncer.Flush(); err != nil {
			t.Fatalf("Flush = %v", err)
		}
		assertSaved(t, f,
			task.State{NextID: 2, Tasks: []task.Task{{ID: 1, Name: "a"}}},
			task.State{NextID: 3, Tasks: []task.Task{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}},
		)
	})
}
