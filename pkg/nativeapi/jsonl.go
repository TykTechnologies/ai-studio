package nativeapi

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// DefaultMaxLineSize caps one JSONL line.
const DefaultMaxLineSize = 16 << 20

// ErrLineTooLong is returned when a line exceeds the reader's cap.
var ErrLineTooLong = errors.New("nativeapi: jsonl line exceeds the size limit")

// LineReader reads JSONL line by line with a bounded line size. It accepts LF
// and CRLF line endings and a missing trailing newline, and skips blank
// lines.
type LineReader struct {
	r    *bufio.Reader
	max  int
	buf  []byte
	line int
	// midLine: the last Next stopped inside an oversized line, before its
	// line ending.
	midLine bool
}

// NewLineReader reads from r with lines capped at maxLine bytes (excluding the
// line ending); maxLine <= 0 means DefaultMaxLineSize.
func NewLineReader(r io.Reader, maxLine int) *LineReader {
	if maxLine <= 0 {
		maxLine = DefaultMaxLineSize
	}
	return &LineReader{r: bufio.NewReaderSize(r, 64<<10), max: maxLine}
}

// Line is the 1-based number of the last line returned or that failed,
// counting blank lines.
func (l *LineReader) Line() int { return l.line }

// Next returns the next non-blank line without its line ending. The slice is
// only valid until the next call. It returns io.EOF after the last line, and
// ErrLineTooLong (wrapped with the line number) without reading the rest of an
// oversized line into memory; SkipLine then moves past it.
func (l *LineReader) Next() ([]byte, error) {
	l.midLine = false
	for {
		l.buf = l.buf[:0]
		l.line++
		for {
			chunk, err := l.r.ReadSlice('\n')
			// The cap applies to content; leave room for a "\r\n" ending so
			// a partial line is rejected as soon as it cannot fit.
			if len(l.buf)+len(chunk) > l.max+2 {
				l.midLine = err == bufio.ErrBufferFull
				return nil, fmt.Errorf("line %d: %w", l.line, ErrLineTooLong)
			}
			l.buf = append(l.buf, chunk...)
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil && err != io.EOF {
				return nil, err
			}
			eof := err == io.EOF
			if eof && len(l.buf) == 0 {
				l.line--
				return nil, io.EOF
			}
			out := trimEOL(l.buf)
			if len(out) > l.max {
				return nil, fmt.Errorf("line %d: %w", l.line, ErrLineTooLong)
			}
			if len(bytes.TrimSpace(out)) == 0 {
				if eof {
					return nil, io.EOF
				}
				break // blank line: read the next one
			}
			return out, nil
		}
	}
}

// SkipLine discards the rest of the oversized line the last Next rejected
// with ErrLineTooLong, without buffering it, so the next Next reads the line
// after it. It is a no-op when the last Next did not stop inside a line.
func (l *LineReader) SkipLine() error {
	for l.midLine {
		_, err := l.r.ReadSlice('\n')
		switch {
		case err == bufio.ErrBufferFull:
			continue
		case err == nil || err == io.EOF:
			l.midLine = false
		default:
			return err
		}
	}
	return nil
}

func trimEOL(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte("\n"))
	return bytes.TrimSuffix(b, []byte("\r"))
}
