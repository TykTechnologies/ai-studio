package services

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempBrandingStorage keeps branding assets in a temporary directory for
// the test. The default, ./data/branding, is created in the package
// directory, which changes what go test keys its cached results on (the
// directory's mtime and listing) on every run.
func useTempBrandingStorage(t *testing.T) {
	t.Helper()
	SetBrandingStoragePath(t.TempDir())
	t.Cleanup(func() { SetBrandingStoragePath("") })
}

// inScratchDir runs the rest of the test in an empty directory outside the
// repository, for tests of paths relative to the working directory (the
// default ./data/branding). Writing there instead of the package directory
// keeps the package directory unchanged while source-walking tests list it,
// and go test keys cached results on that listing. The directory has a
// fixed name and is removed when the test ends, because go test also keys a
// test's result on the name and state of every directory it changes into: a
// fresh t.TempDir() would never match.
//
// The name carries a hash of the package directory, so it stays the same
// within a checkout (and the cached result reusable) but differs between
// checkouts: worktrees running these tests at the same time do not remove
// each other's directory.
func inScratchDir(t *testing.T) {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "midsommar-services-"+checkoutHash(t)+"-"+strings.ReplaceAll(t.Name(), "/", "_"))
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
}

// checkoutHash is a short hash of the package directory, the working
// directory go test starts the tests in.
func checkoutHash(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(wd))
	return hex.EncodeToString(sum[:4])
}
