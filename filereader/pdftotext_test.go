package filereader

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// withPdftotextLookup resets the once-only lookup and replaces its seams for
// one test, capturing the log.
func withPdftotextLookup(t *testing.T, look func(string) (string, error), stat func(string) (os.FileInfo, error)) *bytes.Buffer {
	t.Helper()
	origLook, origStat := lookPath, statFile
	origWriter, origFlags := log.Writer(), log.Flags()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	lookPath, statFile = look, stat
	pdftotextOnce, pdftotextBin = sync.Once{}, ""
	t.Cleanup(func() {
		lookPath, statFile = origLook, origStat
		log.SetOutput(origWriter)
		log.SetFlags(origFlags)
		pdftotextOnce, pdftotextBin = sync.Once{}, ""
	})
	return &logs
}

func notFound(string) (string, error)    { return "", errors.New("not found") }
func noFile(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

// Without pdftotext every PDF goes to the Go parser, and the choice is logged
// once rather than per upload.
func TestPDF_GoParserFallbackLoggedOnce(t *testing.T) {
	logs := withPdftotextLookup(t, notFound, noFile)
	data, err := os.ReadFile("testdata/sample.pdf")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		text, err := ExtractTextFromPDFUsingPopplerWithOptions("sample.pdf", 10*time.Second, data)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !strings.Contains(text, "Lorem ipsum dolor sit amet") {
			t.Fatalf("Go parser text = %q", text)
		}
	}

	if got := strings.Count(logs.String(), "PDF text extraction uses"); got != 1 {
		t.Errorf("logged the extractor choice %d times, want once:\n%s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "built-in Go parser") || strings.Contains(logs.String(), "WARNING") {
		t.Errorf("log = %q, want one info line naming the built-in parser", logs.String())
	}
}

// With pdftotext on PATH it is run for every PDF; the lookup happens once.
func TestPDF_PdftotextUsedWhenOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a fake pdftotext")
	}
	fake := filepath.Join(t.TempDir(), "pdftotext")
	// pdftotext -layout -nopgbrk <in> <out>: write a marker to <out>.
	script := "#!/bin/sh\nfor last; do :; done\nprintf 'from pdftotext' > \"$last\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	logs := withPdftotextLookup(t, func(string) (string, error) { lookups++; return fake, nil }, noFile)

	for i := 0; i < 2; i++ {
		text, err := ExtractTextFromPDFUsingPopplerWithOptions("x.pdf", 10*time.Second, []byte("%PDF-1.4"))
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		if text != "from pdftotext" {
			t.Fatalf("text = %q, want the pdftotext output", text)
		}
	}
	if lookups != 1 {
		t.Errorf("looked pdftotext up %d times, want once", lookups)
	}
	if got := strings.Count(logs.String(), "PDF text extraction uses "+fake); got != 1 {
		t.Errorf("log = %q, want one line naming %s", logs.String(), fake)
	}
}

// ./pdftotext is used when it is not on PATH but present in the working
// directory.
func TestPDF_LocalPdftotextFallback(t *testing.T) {
	withPdftotextLookup(t, notFound, func(name string) (os.FileInfo, error) {
		if name == "./pdftotext" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	})
	if got := pdftotextPath(); got != "./pdftotext" {
		t.Errorf("pdftotextPath() = %q, want ./pdftotext", got)
	}
}
