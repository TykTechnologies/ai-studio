package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #403: branding storage must not create world-accessible directories,
// saved files must not be world-writable/readable beyond need, and the save
// path must be confined to the storage root.

func TestBrandingStorageDirectoryPermissions(t *testing.T) {
	base := filepath.Join(t.TempDir(), "branding")
	_, err := NewBrandingFileStorage(base)
	require.NoError(t, err)

	info, err := os.Stat(base)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o007, "branding dir must not be world-accessible: %o", info.Mode().Perm())
}

func TestBrandingSaveFilePermissions(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	file := newMockFile([]byte("png-bytes"))
	err := storage.saveFile(file, filepath.Join(tempDir, "logo.png"))
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(tempDir, "logo.png"))
	require.NoError(t, err)
	perm := info.Mode().Perm()
	assert.Zero(t, perm&0o007, "saved file must not be world-accessible: %o", perm)
	assert.Zero(t, perm&0o022, "saved file must not be group/world-writable: %o", perm)
}

func TestBrandingSaveFileConfinedToBase(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	outside := filepath.Join(filepath.Dir(tempDir), "escape.png")
	err := storage.saveFile(newMockFile([]byte("x")), outside)
	assert.Error(t, err, "saving outside the storage root must be rejected")

	traversal := filepath.Join(tempDir, "..", "escape2.png")
	err = storage.saveFile(newMockFile([]byte("x")), traversal)
	assert.Error(t, err, "traversal outside the storage root must be rejected")

	_, statErr := os.Stat(outside)
	assert.True(t, os.IsNotExist(statErr), "no file must be written outside the root")
}
