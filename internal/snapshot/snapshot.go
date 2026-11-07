// Package snapshot syncs repository state with a file.
// See docs/spec/extensions/10-persistence.md.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lemotw/task-api/internal/logging"
	"github.com/lemotw/task-api/internal/task"
)

const (
	eventLoadFailed  = "snapshot.load_failed"
	eventFlushFailed = "snapshot.flush_failed"
)

// ErrCorrupt reports a snapshot file that fails PER-7.
var ErrCorrupt = errors.New("snapshot file is corrupt")

// Source is the state being synced.
type Source interface {
	// Export returns the state and its revision consistently (PER-4), with
	// tasks ordered by ascending id.
	Export() (task.State, uint64)
	// Import replaces the state. It is called before serving requests (PER-5).
	Import(state task.State)
}

// Storage persists state.
type Storage interface {
	// Load returns found=false if there is no snapshot (PER-6), an error
	// wrapping ErrCorrupt if the snapshot is invalid (PER-7), and any other
	// error if the snapshot cannot be read (PER-34).
	Load() (state task.State, found bool, err error)
	// Save atomically replaces the snapshot (PER-11).
	Save(state task.State) error
	// CheckWritable reports whether Save can write to the location (PER-10).
	CheckWritable() error
	// Path returns the snapshot location for log events (PER-33).
	Path() string
}

// Syncer coordinates loading, periodic flushes and the final flush.
type Syncer struct {
	source   Source
	storage  Storage
	interval time.Duration
	log      logging.Logger

	mu    sync.Mutex // serializes writes (PER-14) and guards saved
	saved uint64     // revision of the last state known to be in storage
}

// NewSyncer returns a Syncer. An interval of 0 disables periodic flushes.
func NewSyncer(source Source, storage Storage, interval time.Duration, log logging.Logger) *Syncer {
	return &Syncer{source: source, storage: storage, interval: interval, log: log}
}

// Load restores the state from storage at startup (PER-5 to PER-10).
func (s *Syncer) Load() error {
	state, found, err := s.storage.Load()
	if err != nil {
		return s.loadFailed(err)
	}
	if err := s.storage.CheckWritable(); err != nil {
		return s.loadFailed(err)
	}
	if found {
		s.source.Import(state)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, s.saved = s.source.Export()
	return nil
}

// Run flushes unsaved changes every interval (PER-17 to PER-19). It blocks
// until ctx is cancelled and any write in progress has finished (PER-20).
func (s *Syncer) Run(ctx context.Context) {
	if s.interval <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// A failure is logged by Flush and retried on the next tick.
			_ = s.Flush()
		}
	}
}

// Flush writes unsaved changes, if any. It is the final write at shutdown
// (PER-20, PER-22). A change made during the write stays unsaved (PER-18).
func (s *Syncer) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, revision := s.source.Export()
	if revision == s.saved {
		return nil
	}
	if err := s.storage.Save(state); err != nil {
		s.logFailure(eventFlushFailed, err)
		return fmt.Errorf("save snapshot: %w", err)
	}
	s.saved = revision
	return nil
}

func (s *Syncer) loadFailed(err error) error {
	s.logFailure(eventLoadFailed, err)
	return fmt.Errorf("load snapshot: %w", err)
}

func (s *Syncer) logFailure(event string, err error) {
	s.log.Error(event,
		logging.Field{Key: "path", Value: s.storage.Path()},
		logging.Field{Key: "error", Value: err.Error()},
	)
}
