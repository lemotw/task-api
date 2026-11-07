package logging

import "io"

var _ Sink = (*WriterSink)(nil)

// WriterSink writes events to an io.Writer, such as os.Stdout (LOG-2).
type WriterSink struct {
	w io.Writer
}

// NewWriterSink returns a WriterSink.
func NewWriterSink(w io.Writer) *WriterSink {
	return &WriterSink{w: w}
}

// Write implements Sink.
func (s *WriterSink) Write(line []byte) error {
	_, err := s.w.Write(line)
	return err
}

// Close implements Sink. It does not close the underlying writer.
func (s *WriterSink) Close() error {
	return nil
}
