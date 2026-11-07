// Package snapshot syncs repository state with a file.
// See docs/spec/extensions/10-persistence.md.
package snapshot

import (
	"context"
	"errors"
	"time"

	"github.com/lemo/task-api/internal/logging"
	"github.com/lemo/task-api/internal/task"
)

// ErrCorrupt reports a snapshot file that fails PER-7.
var ErrCorrupt = errors.New("snapshot file is corrupt")

// Source is the state being synced.
type Source interface {
	// Export returns the state and its revision consistently (PER-4).
	Export() (task.State, uint64)
	// Import replaces the state. It is called before serving requests (PER-5).
	Import(state task.State)
}

// Storage persists state.
type Storage interface {
	// Load returns found=false if there is no snapshot (PER-6), and an error
	// wrapping ErrCorrupt if the snapshot is invalid (PER-7).
	Load() (state task.State, found bool, err error)
	// Save atomically replaces the snapshot (PER-11).
	Save(state task.State) error
	// CheckWritable reports whether Save can write to the location (PER-10).
	CheckWritable() error
}

// Syncer coordinates loading, periodic flushes and the final flush.
type Syncer struct {
	source   Source
	storage  Storage
	interval time.Duration
	log      logging.Logger
}

// NewSyncer returns a Syncer. An interval of 0 disables periodic flushes.
func NewSyncer(source Source, storage Storage, interval time.Duration, log logging.Logger) *Syncer {
	panic("not implemented")
}

// Load restores the state from storage at startup (PER-5 to PER-10).
func (s *Syncer) Load() error {
	panic("not implemented")
}

// Run flushes unsaved changes every interval (PER-17 to PER-19). It blocks
// until ctx is cancelled and any write in progress has finished (PER-20).
func (s *Syncer) Run(ctx context.Context) {
	panic("not implemented")
}

// Flush writes unsaved changes, if any. It is the final write at shutdown
// (PER-20, PER-22).
func (s *Syncer) Flush() error {
	panic("not implemented")
}
