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
func withPdftotextLookup(t *testing.T, look func(string) (string, error)) *bytes.Buffer {
	t.Helper()
	origLook := lookPath
	origWriter, origFlags := log.Writer(), log.Flags()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	lookPath = look
	pdftotextOnce, pdftotextBin = sync.Once{}, ""
	t.Cleanup(func() {
		lookPath = origLook
		log.SetOutput(origWriter)
		log.SetFlags(origFlags)
		pdftotextOnce, pdftotextBin = sync.Once{}, ""
	})
	return &logs
}

func notFound(string) (string, error) { return "", errors.New("not found") }

// Without pdftotext every PDF goes to the Go parser, and the choice is logged
// once rather than per upload.
func TestPDF_GoParserFallbackLoggedOnce(t *testing.T) {
	logs := withPdftotextLookup(t, notFound)
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
	logs := withPdftotextLookup(t, func(string) (string, error) { lookups++; return fake, nil })

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

// A pdftotext in the working directory is not used: only PATH counts.
func TestPDF_WorkingDirectoryPdftotextIsIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pdftotext"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	withPdftotextLookup(t, notFound)
	if got := pdftotextPath(); got != "" {
		t.Errorf("pdftotextPath() = %q, want the built-in parser", got)
	}
}

// pdftotext parses uploaded files, so it does not get the host's secrets.
func TestPDF_PdftotextGetsMinimalEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a fake pdftotext")
	}
	t.Setenv("ENCRYPTION_KEY", "host-secret")
	t.Setenv("LC_ALL", "C")
	fake := filepath.Join(t.TempDir(), "pdftotext")
	// Write the environment to <out>, the last argument.
	script := "#!/bin/sh\nfor last; do :; done\nenv > \"$last\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	withPdftotextLookup(t, func(string) (string, error) { return fake, nil })

	env, err := ExtractTextFromPDFUsingPopplerWithOptions("x.pdf", 10*time.Second, []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(env, "ENCRYPTION_KEY") {
		t.Errorf("pdftotext saw ENCRYPTION_KEY:\n%s", env)
	}
	if !strings.Contains(env, "LC_ALL=C") || !strings.Contains(env, "PATH=") {
		t.Errorf("pdftotext is missing PATH or locale:\n%s", env)
	}
}
