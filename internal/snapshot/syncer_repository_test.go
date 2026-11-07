package snapshot

import (
	"sync"
	"testing"
	"time"

	"github.com/lemo/task-api/internal/logging/logtest"
	"github.com/lemo/task-api/internal/memory"
	"github.com/lemo/task-api/internal/task"
)

// A slow file write must not block API requests (PER-15).
func TestFlush_RepositoryStaysUsableDuringWrite(t *testing.T) {
	const timeout = 5 * time.Second

	repo := memory.New()
	storage := &fakeStorage{}
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSave := func() { releaseOnce.Do(func() { close(release) }) }
	storage.saveFn = func(task.State) error {
		close(entered)
		<-release
		return nil
	}

	syncer := NewSyncer(repo, storage, 0, &logtest.Recorder{})
	if err := syncer.Load(); err != nil {
		t.Fatalf("Load = %v", err)
	}
	repo.Create("a", task.StatusIncomplete)

	flushed := make(chan error, 1)
	go func() { flushed <- syncer.Flush() }()
	created := make(chan task.Task, 1)

	// Every failure path unblocks Save. The channels are buffered, so both
	// goroutines can finish even if nothing reads their results.
	defer releaseSave()

	select {
	case <-entered:
	case err := <-flushed:
		t.Fatalf("Flush returned %v without calling Save", err)
	case <-time.After(timeout):
		t.Fatal("Flush did not reach Save")
	}

	go func() { created <- repo.Create("b", task.StatusIncomplete) }()
	select {
	case got := <-created:
		if got.ID != 2 {
			t.Errorf("id = %d", got.ID)
		}
	case <-time.After(timeout):
		releaseSave()
		select { // the blocked Create finishes once the write ends
		case <-created:
		case <-time.After(timeout):
		}
		t.Fatal("Create blocked while the snapshot was being written")
	}

	releaseSave()
	select {
	case err := <-flushed:
		if err != nil {
			t.Fatalf("Flush = %v", err)
		}
	case <-time.After(timeout):
		t.Fatal("Flush did not finish after Save was released")
	}
}
