package ociplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Issue #403: storage directories must be 0750, stored files must not be
// world-readable, and digest/arch inputs must be confined to the storage root.

func newTestStorage(t *testing.T) *ContentStorage {
	t.Helper()
	s, err := NewContentStorage(t.TempDir(), 1<<30, time.Hour)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	return s
}

func digestOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestStorageDirectoryPermissions(t *testing.T) {
	s := newTestStorage(t)

	for _, dir := range []string{s.getCASDir(), s.getBinsDir(), s.getActiveDir(), s.getTempDir(), s.getMetadataDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm&0o007 != 0 {
			t.Errorf("directory %s is world-accessible: %o", dir, perm)
		}
	}
}

func TestStoredBlobNotWorldReadable(t *testing.T) {
	s := newTestStorage(t)

	digest, err := s.StoreBlob([]byte("plugin-content"))
	if err != nil {
		t.Fatalf("StoreBlob: %v", err)
	}

	info, err := os.Stat(filepath.Join(s.getCASDir(), digest))
	if err != nil {
		t.Fatalf("stat blob: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("blob is group/world-accessible: %o", perm)
	}
}

func TestStoredExecutablePermissions(t *testing.T) {
	s := newTestStorage(t)

	data := []byte("#!/bin/sh\nexit 0\n")
	path, err := s.StoreExecutable(digestOf(data), "linux/amd64", data)
	if err != nil {
		t.Fatalf("StoreExecutable: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat exec: %v", err)
	}
	perm := info.Mode().Perm()
	if perm&0o100 == 0 {
		t.Errorf("executable is not owner-executable: %o", perm)
	}
	if perm&0o007 != 0 {
		t.Errorf("executable is world-accessible: %o", perm)
	}
}

func TestGetBlobRejectsTraversalDigest(t *testing.T) {
	s := newTestStorage(t)

	// Plant a file outside the CAS dir that a traversal digest would reach
	outside := filepath.Join(s.baseDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, digest := range []string{
		"../../secret.txt",
		"..",
		"",
		"SHA256-UPPER-NOT-HEX",
		strings.Repeat("a", 63), // wrong length
	} {
		if _, err := s.GetBlob(digest); err == nil {
			t.Errorf("GetBlob(%q) should be rejected", digest)
		}
	}
}

func TestStoreExecutableRejectsBadInputs(t *testing.T) {
	s := newTestStorage(t)
	data := []byte("bin")

	if _, err := s.StoreExecutable("../../evil", "linux/amd64", data); err == nil {
		t.Error("traversal digest must be rejected")
	}
	if _, err := s.StoreExecutable(digestOf(data), "../..", data); err == nil {
		t.Error("traversal arch must be rejected")
	}
}

func TestMetadataRejectsTraversal(t *testing.T) {
	s := newTestStorage(t)

	meta := &PluginMetadata{Version: "1.0"}
	if err := s.StoreMetadata("../../evil", "linux/amd64", meta); err == nil {
		t.Error("traversal digest must be rejected in StoreMetadata")
	}
	if _, err := s.LoadMetadata("../../evil", "linux/amd64"); err == nil {
		t.Error("traversal digest must be rejected in LoadMetadata")
	}

	// Valid round-trip still works
	data := []byte("x")
	d := digestOf(data)
	if err := s.StoreMetadata(d, "linux/amd64", meta); err != nil {
		t.Fatalf("StoreMetadata valid: %v", err)
	}
	got, err := s.LoadMetadata(d, "linux/amd64")
	if err != nil {
		t.Fatalf("LoadMetadata valid: %v", err)
	}
	if got.Version != "1.0" {
		t.Errorf("metadata round-trip mangled: %+v", got)
	}
}

// OCI references carry digests as "sha256:<hex>" (the marketplace builds
// "oci://registry/repo@sha256:..."), so storage must accept that spelling and
// map it onto the same on-disk entry as the bare hex form.
func TestStorageAcceptsPrefixedDigest(t *testing.T) {
	s := newTestStorage(t)

	data := []byte("plugin-binary")
	bare := digestOf(data)
	prefixed := "sha256:" + bare

	execPath, err := s.StoreExecutable(prefixed, "linux/amd64", data)
	if err != nil {
		t.Fatalf("StoreExecutable with prefixed digest: %v", err)
	}
	if got := s.GetExecutablePath(bare, "linux/amd64"); got != execPath {
		t.Errorf("prefixed and bare digests map to different paths: %q vs %q", got, execPath)
	}
	if !s.HasExecutable(prefixed, "linux/amd64") || !s.HasExecutable(bare, "linux/amd64") {
		t.Error("executable must be found under both digest spellings")
	}
	if filepath.Base(execPath) != "sha256-"+bare+"-linux-amd64" {
		t.Errorf("unexpected executable filename: %s", filepath.Base(execPath))
	}

	meta := &PluginMetadata{Version: "1.0"}
	if err := s.StoreMetadata(prefixed, "linux/amd64", meta); err != nil {
		t.Fatalf("StoreMetadata with prefixed digest: %v", err)
	}
	if !s.HasMetadata(bare, "linux/amd64") {
		t.Error("metadata stored under prefixed digest must be found under bare digest")
	}

	if _, err := s.StoreBlob(data); err != nil {
		t.Fatalf("StoreBlob: %v", err)
	}
	if _, err := s.GetBlob(prefixed); err != nil {
		t.Errorf("GetBlob with prefixed digest: %v", err)
	}
}

// The prefix must not become a way around the path checks.
func TestStorageRejectsPrefixedTraversal(t *testing.T) {
	s := newTestStorage(t)

	for _, digest := range []string{
		"sha256:../../secret.txt",
		"sha256:",
		"sha256:sha256:" + strings.Repeat("a", 64),
	} {
		if _, err := s.GetBlob(digest); err == nil {
			t.Errorf("GetBlob(%q) should be rejected", digest)
		}
		if _, err := s.StoreExecutable(digest, "linux/amd64", []byte("x")); err == nil {
			t.Errorf("StoreExecutable(%q) should be rejected", digest)
		}
		if path := s.GetExecutablePath(digest, "linux/amd64"); path != "" {
			t.Errorf("GetExecutablePath(%q) returned %q, want empty", digest, path)
		}
	}
}
