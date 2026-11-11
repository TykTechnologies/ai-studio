package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

var testPNG = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3}

// forEachBrandingService runs body on SQLite and, with DATABASE_URL, on
// Postgres. The service's BRANDING_STORAGE_PATH is an empty directory.
func forEachBrandingService(t *testing.T, body func(t *testing.T, s *Service, dir string)) {
	t.Run("sqlite", func(t *testing.T) {
		s, _ := setupBrandingTest(t)
		body(t, s, GetBrandingStoragePath())
	})
	t.Run("postgres", func(t *testing.T) {
		s := setupIsolatedPostgresService(t)
		useTempBrandingStorage(t)
		body(t, s, GetBrandingStoragePath())
	})
}

func uploadLogo(t *testing.T, s *Service, data []byte, contentType string) *models.BrandingSettings {
	t.Helper()
	settings, err := s.UploadLogo(newMockFile(data), createMockFileHeader("anything", contentType, int64(len(data))), true)
	require.NoError(t, err)
	return settings
}

// expireBrandingCache makes the service's copies due for a hash check.
func expireBrandingCache(s *Service) {
	s.brandingAssets.mu.Lock()
	defer s.brandingAssets.mu.Unlock()
	for kind, e := range s.brandingAssets.entries {
		e.checked = time.Now().Add(-2 * brandingCacheTTL)
		s.brandingAssets.entries[kind] = e
	}
}

// An upload is stored in the database with its content type and hash, and
// nothing is written to the storage directory. A second replica (another
// Service on the same database) serves it.
func TestBrandingAssetsStoredInDatabase_Postgres(t *testing.T) {
	forEachBrandingService(t, func(t *testing.T, a *Service, dir string) {
		b := NewService(a.DB)
		none, err := b.BrandingLogo()
		require.NoError(t, err)
		assert.Nil(t, none, "nothing uploaded yet")

		settings := uploadLogo(t, a, testPNG, "image/png")
		assert.Equal(t, "logo.png", settings.LogoPath)
		assert.True(t, settings.HasCustomLogo())

		sum := sha256.Sum256(testPNG)
		for name, replica := range map[string]*Service{"the uploading replica": a, "a second replica after the signal": b} {
			if replica == b {
				b.InvalidateBrandingAssets() // what replicas.SignalBranding does
			}
			logo, err := replica.BrandingLogo()
			require.NoError(t, err, name)
			require.NotNil(t, logo, name)
			assert.Equal(t, testPNG, logo.Data, name)
			assert.Equal(t, "image/png", logo.ContentType)
			assert.Equal(t, hex.EncodeToString(sum[:]), logo.Hash)
			assert.Equal(t, int64(len(testPNG)), logo.Size)
		}
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "no file is written")

		// A favicon is a row of its own, served as image/x-icon whichever
		// of the two icon types the browser declared.
		ico := []byte{0, 0, 1, 0, 9}
		fav, err := a.UploadFavicon(newMockFile(ico), createMockFileHeader("f", "image/vnd.microsoft.icon", int64(len(ico))), true)
		require.NoError(t, err)
		assert.Equal(t, "favicon.ico", fav.FaviconPath)
		assert.Equal(t, "logo.png", fav.LogoPath, "the logo is kept")
		favicon, err := a.BrandingFavicon()
		require.NoError(t, err)
		require.NotNil(t, favicon)
		assert.Equal(t, "image/x-icon", favicon.ContentType)
		assert.Equal(t, ico, favicon.Data)

		// A new logo replaces the old one. A replica that missed the signal
		// notices at its next hash check.
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
		settings = uploadLogo(t, a, svg, "image/svg+xml")
		assert.Equal(t, "logo.svg", settings.LogoPath)
		stale, err := b.BrandingLogo()
		require.NoError(t, err)
		assert.Equal(t, testPNG, stale.Data, "within the cache period, without the signal")
		expireBrandingCache(b)
		fresh, err := b.BrandingLogo()
		require.NoError(t, err)
		assert.Equal(t, svg, fresh.Data)
		assert.Equal(t, "image/svg+xml", fresh.ContentType)
		var rows int64
		require.NoError(t, a.DB.Model(&models.BrandingAsset{}).Count(&rows).Error)
		assert.Equal(t, int64(2), rows, "one row per kind")

		// Reset clears the rows and the settings.
		_, err = a.ResetBrandingToDefaults(false)
		assert.ErrorIs(t, err, ErrUnauthorized)
		settings, err = a.ResetBrandingToDefaults(true)
		require.NoError(t, err)
		assert.False(t, settings.HasCustomLogo())
		assert.False(t, settings.HasCustomFavicon())
		require.NoError(t, a.DB.Model(&models.BrandingAsset{}).Count(&rows).Error)
		assert.Zero(t, rows)
		logo, err := a.BrandingLogo()
		require.NoError(t, err)
		assert.Nil(t, logo)
		expireBrandingCache(b)
		logo, err = b.BrandingLogo()
		require.NoError(t, err)
		assert.Nil(t, logo, "the other replica notices the reset")
	})
}

// Size and type are checked as the file storage checked them, and the size
// limit holds for the bytes read, whatever the header declares.
func TestBrandingUploadValidation(t *testing.T) {
	s, db := setupBrandingTest(t)

	_, err := s.UploadLogo(newMockFile(testPNG), createMockFileHeader("l", "image/png", int64(len(testPNG))), false)
	assert.ErrorIs(t, err, ErrUnauthorized)

	_, err = s.UploadLogo(newMockFile(testPNG), createMockFileHeader("l", "image/png", MaxLogoSize+1), true)
	assert.Equal(t, ErrFileTooLarge, err, "declared size over the limit")
	big := bytes.Repeat([]byte{1}, MaxLogoSize+1)
	_, err = s.UploadLogo(newMockFile(big), createMockFileHeader("l", "image/png", 10), true)
	assert.Equal(t, ErrFileTooLarge, err, "more bytes than declared")
	exact := bytes.Repeat([]byte{1}, MaxLogoSize)
	_, err = s.UploadLogo(newMockFile(exact), createMockFileHeader("l", "image/png", MaxLogoSize), true)
	assert.NoError(t, err, "exactly the limit")

	bigIcon := bytes.Repeat([]byte{1}, MaxFaviconSize+1)
	_, err = s.UploadFavicon(newMockFile(bigIcon), createMockFileHeader("f", "image/png", 10), true)
	assert.Equal(t, ErrFileTooLarge, err)

	for _, bad := range []string{"image/gif", "text/html", "", "image/x-icon"} {
		_, err = s.UploadLogo(newMockFile(testPNG), createMockFileHeader("l", bad, int64(len(testPNG))), true)
		assert.Equal(t, ErrInvalidFileType, err, bad)
	}
	_, err = s.UploadFavicon(newMockFile(testPNG), createMockFileHeader("f", "image/svg+xml", int64(len(testPNG))), true)
	assert.Equal(t, ErrInvalidFileType, err)
	_, err = s.UploadFavicon(newMockFile(nil), createMockFileHeader("f", "image/png", 0), true)
	assert.Error(t, err, "an empty file")

	var rows int64
	require.NoError(t, db.Model(&models.BrandingAsset{}).Count(&rows).Error)
	assert.Equal(t, int64(1), rows, "only the valid logo was stored")
}

// While the database cannot be read, a replica keeps serving the copy it
// holds.
func TestBrandingAssetServedFromMemoryWithoutDatabase(t *testing.T) {
	s, db := setupBrandingTest(t)
	uploadLogo(t, s, testPNG, "image/png")
	_, err := s.BrandingLogo()
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	expireBrandingCache(s)
	logo, err := s.BrandingLogo()
	require.NoError(t, err)
	require.NotNil(t, logo)
	assert.Equal(t, testPNG, logo.Data)

	_, err = NewService(db).BrandingLogo()
	assert.Error(t, err, "a replica with no copy reports the failure")
}

// Assets uploaded before they were stored in the database are files under
// BRANDING_STORAGE_PATH on the replica that took the upload. They are
// imported once at start, the files left in place, and never again after a
// reset.
func TestImportBrandingFiles_Postgres(t *testing.T) {
	forEachBrandingService(t, func(t *testing.T, s *Service, dir string) {
		assert.Empty(t, s.ImportBrandingFiles(dir), "default branding: nothing to import")

		// The state an upgraded installation is in.
		ico := []byte{0, 0, 1, 0, 7}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "logo.png"), testPNG, 0o640))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.ico"), ico, 0o640))
		settings, err := s.GetBrandingSettings()
		require.NoError(t, err)
		settings.LogoPath, settings.FaviconPath = "logo.png", "favicon.ico"
		require.NoError(t, settings.Update(s.DB))

		// A replica in the other site has no files: nothing happens there.
		other := NewService(s.DB)
		assert.Empty(t, other.ImportBrandingFiles(t.TempDir()))
		assert.Empty(t, other.ImportBrandingFiles(filepath.Join(dir, "does-not-exist")))
		none, err := other.BrandingLogo()
		require.NoError(t, err)
		assert.Nil(t, none)

		assert.Equal(t, []string{models.BrandingAssetLogo, models.BrandingAssetFavicon}, s.ImportBrandingFiles(dir))
		logo, err := s.BrandingLogo()
		require.NoError(t, err)
		require.NotNil(t, logo)
		assert.Equal(t, testPNG, logo.Data)
		assert.Equal(t, "image/png", logo.ContentType)
		favicon, err := s.BrandingFavicon()
		require.NoError(t, err)
		require.NotNil(t, favicon)
		assert.Equal(t, ico, favicon.Data)
		assert.Equal(t, "image/x-icon", favicon.ContentType)
		for _, name := range []string{"logo.png", "favicon.ico"} {
			_, err := os.Stat(filepath.Join(dir, name))
			assert.NoError(t, err, "the file is left in place")
		}

		// Once only: not again at the next start, not over a newer upload,
		// and not after a reset.
		assert.Empty(t, s.ImportBrandingFiles(dir))
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
		uploadLogo(t, s, svg, "image/svg+xml")
		assert.Empty(t, s.ImportBrandingFiles(dir))
		logo, err = s.BrandingLogo()
		require.NoError(t, err)
		assert.Equal(t, svg, logo.Data)
		_, err = s.ResetBrandingToDefaults(true)
		require.NoError(t, err)
		assert.Empty(t, s.ImportBrandingFiles(dir))
		logo, err = s.BrandingLogo()
		require.NoError(t, err)
		assert.Nil(t, logo, "a reset is not undone by the old file")
	})
}

// What the import refuses: a name that leaves the directory, a file type
// the asset cannot have, and a file over the size limit.
func TestImportBrandingFilesRefusals(t *testing.T) {
	s, _ := setupBrandingTest(t)
	dir := GetBrandingStoragePath()
	outside := filepath.Join(filepath.Dir(dir), "outside.png")
	require.NoError(t, os.WriteFile(outside, testPNG, 0o640))
	t.Cleanup(func() { os.Remove(outside) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "logo.html"), []byte("<html>"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.png"), bytes.Repeat([]byte{1}, MaxFaviconSize+1), 0o640))

	set := func(logo, favicon string) {
		t.Helper()
		settings, err := s.GetBrandingSettings()
		require.NoError(t, err)
		settings.LogoPath, settings.FaviconPath = logo, favicon
		require.NoError(t, settings.Update(s.DB))
	}
	set("../outside.png", "favicon.png")
	assert.Empty(t, s.ImportBrandingFiles(dir))
	set("logo.html", "")
	assert.Empty(t, s.ImportBrandingFiles(dir))

	var rows int64
	require.NoError(t, s.DB.Model(&models.BrandingAsset{}).Count(&rows).Error)
	assert.Zero(t, rows)
}
