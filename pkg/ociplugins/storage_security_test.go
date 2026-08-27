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
