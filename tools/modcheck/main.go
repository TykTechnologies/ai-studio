// Command modcheck builds the root module's zip from a git revision the way
// the Go module proxy does, and fails if the proxy would refuse it.
//
// A single committed file the zip format rejects (a path holding ':', a
// case-insensitive name clash, an oversized go.mod) makes every version of
// github.com/TykTechnologies/midsommar/v2 impossible to `go get`, and nothing
// else notices: the repository builds and tests fine from a checkout. Run it
// with `make module-check`; it checks HEAD, so commit first.
//
// It is its own module so that golang.org/x/mod stays out of the root
// module's requirements.
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func main() {
	rev := flag.String("rev", "HEAD", "git revision to build the module zip from")
	maxMB := flag.Int64("max-mb", 150, "fail when the zip is larger than this many MiB (the proxy's own limit is 500)")
	flag.Parse()

	files, err := archiveFiles(*rev)
	if err != nil {
		fail("%v", err)
	}

	m := module.Version{Path: "github.com/TykTechnologies/midsommar/v2", Version: "v2.999.999"}
	var w countingWriter
	if err := modzip.Create(&w, m, files); err != nil {
		fail("the module proxy would refuse %s:\n%v", *rev, err)
	}

	mb := float64(w.n) / (1 << 20)
	if w.n > *maxMB<<20 {
		fail("module zip is %.1f MiB, over the %d MiB budget: a committed binary or data file is likely", mb, *maxMB)
	}
	fmt.Printf("module zip OK: %.1f MiB, %d files at %s\n", mb, len(files), *rev)
}

// archiveFiles lists what the proxy sees at rev: the go command builds module
// zips from `git archive`, which leaves out untracked and ignored files and
// the contents of submodules. modzip.CreateFromVCS does the same but refuses
// git worktrees, hence this copy of it.
func archiveFiles(rev string) ([]modzip.File, error) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-c", "core.autocrlf=input", "-c", "core.eol=lf", "archive", "--format=zip", rev)
	cmd.Dir = strings.TrimSpace(string(top))
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git archive %s: %w: %s", rev, err, stderr.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(stdout.Bytes()), int64(stdout.Len()))
	if err != nil {
		return nil, err
	}
	var files []modzip.File
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, "/") {
			files = append(files, archiveFile{f})
		}
	}
	return files, nil
}

type archiveFile struct{ f *zip.File }

func (a archiveFile) Path() string                 { return a.f.Name }
func (a archiveFile) Lstat() (os.FileInfo, error)  { return a.f.FileInfo(), nil }
func (a archiveFile) Open() (io.ReadCloser, error) { return a.f.Open() }

// countingWriter counts the bytes of the zip without keeping them.
type countingWriter struct{ n int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	return len(p), nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "modcheck: "+format+"\n", args...)
	os.Exit(1)
}
