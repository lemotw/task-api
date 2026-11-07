package logging

import "io"

var _ Sink = (*WriterSink)(nil)

// WriterSink writes events to an io.Writer, such as os.Stdout (LOG-2).
type WriterSink struct {
	w io.Writer
}

// NewWriterSink returns a WriterSink.
func NewWriterSink(w io.Writer) *WriterSink {
	panic("not implemented")
}

// Write implements Sink.
func (s *WriterSink) Write(line []byte) error {
	panic("not implemented")
}

// Close implements Sink. It does not close the underlying writer.
func (s *WriterSink) Close() error {
	panic("not implemented")
}
