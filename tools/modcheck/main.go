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
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func main() {
	rev := flag.String("rev", "HEAD", "git revision to build the module zip from")
	maxMB := flag.Int64("max-mb", 150, "fail when the zip is larger than this many MiB (the proxy's own limit is 500)")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fail("%v", err)
	}
	modPath, err := modulePath(root)
	if err != nil {
		fail("%v", err)
	}
	files, err := archiveFiles(root, *rev)
	if err != nil {
		fail("%v", err)
	}

	// The version only has to be valid for the module path's major version.
	m := module.Version{Path: modPath, Version: pseudoVersion(modPath)}
	var w countingWriter
	if err := modzip.Create(&w, m, files); err != nil {
		fail("the module proxy would refuse %s:\n%v", *rev, err)
	}

	mb := float64(w.n) / (1 << 20)
	if w.n > *maxMB<<20 {
		fail("module zip is %.1f MiB, over the %d MiB budget: a committed binary or data file is likely", mb, *maxMB)
	}
	fmt.Printf("module zip OK: %s, %.1f MiB, %d files at %s\n", modPath, mb, len(files), *rev)
}

// archiveFiles lists what the proxy sees at rev: the go command builds module
// zips from `git archive`, which leaves out untracked and ignored files and
// the contents of submodules. modzip.CreateFromVCS does the same but refuses
// git worktrees, hence this copy of it.
func archiveFiles(root, rev string) ([]modzip.File, error) {
	// Resolve rev to a tree hash first, so that -rev can only name a
	// revision: --end-of-options stops a value such as "--output=..." being
	// read as an option, and git archive then only ever sees a hash.
	tree, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--end-of-options", rev+"^{tree}").Output()
	if err != nil {
		return nil, fmt.Errorf("%q is not a git revision: %w", rev, err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-c", "core.autocrlf=input", "-c", "core.eol=lf", "archive", "--format=zip", strings.TrimSpace(string(tree)))
	cmd.Dir = root
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

func repoRoot() (string, error) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(string(top)), nil
}

// modulePath reads the root module's path from its go.mod.
func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	if p := modfile.ModulePath(data); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("no module line in %s/go.mod", root)
}

// pseudoVersion returns a version that is valid for the module path's major
// version (v2.999.999 for a /v2 path), which is all modzip.Create checks.
func pseudoVersion(modPath string) string {
	if _, major, ok := module.SplitPathVersion(modPath); ok && major != "" {
		return strings.TrimPrefix(major, "/") + ".999.999"
	}
	return "v0.999.999"
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
