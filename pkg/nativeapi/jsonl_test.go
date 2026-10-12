package nativeapi

import (
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"
)

func mustRe(s string) *regexp.Regexp { return regexp.MustCompile(s) }

func readAll(t *testing.T, r io.Reader, max int) ([]string, error) {
	t.Helper()
	lr := NewLineReader(r, max)
	var out []string
	for {
		line, err := lr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, string(line))
	}
}

func TestLineReader(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"lf", "{\"a\":1}\n{\"b\":2}\n", []string{`{"a":1}`, `{"b":2}`}},
		{"crlf", "{\"a\":1}\r\n{\"b\":2}\r\n", []string{`{"a":1}`, `{"b":2}`}},
		{"no trailing newline", "{\"a\":1}\n{\"b\":2}", []string{`{"a":1}`, `{"b":2}`}},
		{"no trailing newline crlf", "{\"a\":1}\r\n{\"b\":2}\r", []string{`{"a":1}`, `{"b":2}`}},
		{"blank lines skipped", "\n\r\n{\"a\":1}\n   \n\t\r\n{\"b\":2}\n\n", []string{`{"a":1}`, `{"b":2}`}},
		{"trailing blank no newline", "{\"a\":1}\n  ", []string{`{"a":1}`}},
		{"empty", "", nil},
		{"only blanks", "\n\n\r\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readAll(t, strings.NewReader(tc.in), 0)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLineReaderLongLines(t *testing.T) {
	// Lines far beyond the bufio buffer, under the cap, read whole even when
	// the source returns one byte at a time.
	long := strings.Repeat("x", 300<<10)
	in := long + "\r\n" + long
	got, err := readAll(t, iotest.OneByteReader(strings.NewReader(in)), len(long))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != long || got[1] != long {
		t.Fatalf("got %d lines", len(got))
	}
}

func TestLineReaderOverCap(t *testing.T) {
	for _, in := range []string{
		"ok\n" + strings.Repeat("x", 11) + "\n",
		"ok\n" + strings.Repeat("x", 11),             // at EOF
		"ok\n" + strings.Repeat("x", 11) + "\r\n",    // CRLF does not count, content does
		"ok\n" + strings.Repeat("x", 200<<10) + "\n", // spans many buffer fills
	} {
		lr := NewLineReader(strings.NewReader(in), 10)
		if line, err := lr.Next(); err != nil || string(line) != "ok" {
			t.Fatalf("first line = %q, %v", line, err)
		}
		_, err := lr.Next()
		if !errors.Is(err, ErrLineTooLong) {
			t.Fatalf("err = %v, want ErrLineTooLong", err)
		}
		if lr.Line() != 2 || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("line = %d, err %v", lr.Line(), err)
		}
	}
	// Exactly at the cap, with either ending, is fine.
	for _, end := range []string{"\n", "\r\n", ""} {
		got, err := readAll(t, strings.NewReader(strings.Repeat("y", 10)+end), 10)
		if err != nil || len(got) != 1 {
			t.Fatalf("at-cap line with ending %q: %v, %v", end, got, err)
		}
	}
}

func TestLineReaderDefaultCap(t *testing.T) {
	lr := NewLineReader(strings.NewReader(""), 0)
	if lr.max != DefaultMaxLineSize {
		t.Fatalf("max = %d", lr.max)
	}
}

func TestLineReaderPropagatesReadErrors(t *testing.T) {
	boom := errors.New("boom")
	lr := NewLineReader(iotest.ErrReader(boom), 0)
	if _, err := lr.Next(); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

// SkipLine moves past an oversized line without buffering it, whether the
// cap was hit mid-line or at the line's end, and Next resumes on the next
// line with the line count intact.
func TestLineReaderSkipLine(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     []string
		over     int
	}{
		{"mid line", "a\n" + strings.Repeat("x", 300<<10) + "\nb\n", []string{"a", "b"}, 1},
		{"crlf", "a\r\n" + strings.Repeat("x", 300<<10) + "\r\nb\r\n", []string{"a", "b"}, 1},
		{"just over at end of line", "a\n" + strings.Repeat("x", 11) + "\nb\n", []string{"a", "b"}, 1},
		{"over at eof", "a\n" + strings.Repeat("x", 300<<10), []string{"a"}, 1},
		{"two in a row", "a\n" + strings.Repeat("x", 100) + "\n" + strings.Repeat("y", 100) + "\nb", []string{"a", "b"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			max := 10
			if strings.Contains(tc.name, "mid") || strings.Contains(tc.name, "crlf") || strings.Contains(tc.name, "eof") {
				max = 1 << 10
			}
			lr := NewLineReader(iotest.HalfReader(strings.NewReader(tc.in)), max)
			var got []string
			over := 0
			for {
				line, err := lr.Next()
				if err == io.EOF {
					break
				}
				if errors.Is(err, ErrLineTooLong) {
					over++
					if err := lr.SkipLine(); err != nil {
						t.Fatal(err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, string(line))
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || over != tc.over {
				t.Fatalf("got %q (%d oversized), want %q (%d)", got, over, tc.want, tc.over)
			}
		})
	}
	// A no-op when nothing was rejected.
	lr := NewLineReader(strings.NewReader("a\nb\n"), 0)
	_, _ = lr.Next()
	if err := lr.SkipLine(); err != nil {
		t.Fatal(err)
	}
	if line, err := lr.Next(); err != nil || string(line) != "b" {
		t.Fatalf("after no-op SkipLine: %q %v", line, err)
	}
}
