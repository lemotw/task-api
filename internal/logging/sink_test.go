package logging

import (
	"bytes"
	"errors"
	"testing"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type closableWriter struct {
	bytes.Buffer
	closed bool
}

func (w *closableWriter) Close() error {
	w.closed = true
	return nil
}

func TestWriterSink_WritesLineUnchanged(t *testing.T) {
	var buf bytes.Buffer
	s := NewWriterSink(&buf)
	if err := s.Write([]byte("{\"a\":1}\n")); err != nil {
		t.Fatalf("Write = %v", err)
	}
	if got := buf.String(); got != "{\"a\":1}\n" {
		t.Errorf("wrote %q", got)
	}
}

func TestWriterSink_ReturnsWriterError(t *testing.T) {
	want := errors.New("write failed")
	s := NewWriterSink(failingWriter{err: want})
	if err := s.Write([]byte("x")); !errors.Is(err, want) {
		t.Fatalf("Write = %v", err)
	}
}

func TestWriterSink_CloseLeavesWriterOpen(t *testing.T) {
	w := &closableWriter{}
	s := NewWriterSink(w)
	if err := s.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if w.closed {
		t.Error("underlying writer was closed")
	}
}
