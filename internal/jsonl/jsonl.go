// Package jsonl reads newline-delimited JSON logs of any length, such as the
// session logs coding agents write, one decoded record at a time.
package jsonl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
)

// DecodeError reports a line that is not a valid record. Agents may leave a
// partial last line while a session is still running, so callers usually
// count these and carry on.
type DecodeError struct {
	Line int
	Err  error
}

func (e *DecodeError) Error() string { return fmt.Sprintf("line %d: %v", e.Line, e.Err) }
func (e *DecodeError) Unwrap() error { return e.Err }

// MaxLine is the longest line Records decodes. Lines holding screenshots run
// to megabytes; one far longer is more likely a corrupt or foreign file,
// and reading it whole, in every worker at once, could exhaust memory.
const MaxLine = 64 << 20

// ErrLineTooLong is the DecodeError cause for a line over MaxLine, which is
// skipped unread.
var ErrLineTooLong = fmt.Errorf("line longer than %d MiB", MaxLine>>20)

// Records yields each line of r decoded as a T. A line that fails to decode
// yields a *DecodeError and iteration continues. A read error is yielded once
// and ends iteration. Blank lines are skipped.
func Records[T any](r io.Reader) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		lr := newLineReader(r)
		for n := 1; ; n++ {
			line, err := lr.next()
			if errors.Is(err, ErrLineTooLong) {
				var zero T
				if !yield(zero, &DecodeError{Line: n, Err: err}) {
					return
				}
				continue
			}
			if len(bytes.TrimSpace(line)) > 0 {
				var rec T
				if decErr := json.Unmarshal(line, &rec); decErr != nil {
					if !yield(rec, &DecodeError{Line: n, Err: decErr}) {
						return
					}
				} else if !yield(rec, nil) {
					return
				}
			}
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				var zero T
				yield(zero, fmt.Errorf("reading log: %w", err))
				return
			}
		}
	}
}

// lineReader returns lines of any length, reusing one buffer for lines that
// exceed bufio's internal buffer. Transcript lines can be tens of megabytes.
type lineReader struct {
	r   *bufio.Reader
	buf []byte
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReaderSize(r, 256<<10)}
}

// next returns the next line without its newline. The slice is valid until
// the following call. A line over MaxLine is read past and discarded, and
// next returns ErrLineTooLong for it.
func (l *lineReader) next() ([]byte, error) {
	l.buf = l.buf[:0]
	for {
		chunk, err := l.r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			if len(l.buf)+len(chunk) > MaxLine {
				return nil, l.skip()
			}
			l.buf = append(l.buf, chunk...)
			continue
		}
		if len(l.buf)+len(chunk) > MaxLine+1 && (err == nil || errors.Is(err, io.EOF)) {
			// At the end of the file the next call reports io.EOF.
			l.buf = nil
			return nil, ErrLineTooLong
		}
		var line []byte
		if len(l.buf) > 0 {
			l.buf = append(l.buf, chunk...)
			line = l.buf
		} else {
			line = chunk
		}
		return bytes.TrimSuffix(line, []byte{'\n'}), err
	}
}

// skip reads to the end of the current line and frees the buffer, so one
// huge line does not keep its memory for the rest of the file.
func (l *lineReader) skip() error {
	l.buf = nil
	for {
		_, err := l.r.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err == nil, errors.Is(err, io.EOF):
			return ErrLineTooLong
		default:
			return err
		}
	}
}
