package services

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFile implements multipart.File for testing
type mockFile struct {
	*bytes.Reader
}

func (m *mockFile) Close() error {
	return nil
}

func (m *mockFile) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, io.EOF
}

func newMockFile(data []byte) multipart.File {
	return &mockFile{Reader: bytes.NewReader(data)}
}

func setupBrandingStorageTest(t *testing.T) (*BrandingFileStorage, string) {
	// Create temp directory for test
	tempDir := t.TempDir()

	storage, err := NewBrandingFileStorage(tempDir)
	assert.NoError(t, err)

	return storage, tempDir
}

func createMockFileHeader(filename, contentType string, size int64) *multipart.FileHeader {
	header := &multipart.FileHeader{
		Filename: filename,
		Size:     size,
	}
	header.Header = textproto.MIMEHeader{}
	header.Header.Set("Content-Type", contentType)
	return header
}

func TestNewBrandingFileStorage(t *testing.T) {
	t.Run("Create with custom path", func(t *testing.T) {
		tempDir := t.TempDir()
		storage, err := NewBrandingFileStorage(tempDir)
		assert.NoError(t, err)
		assert.NotNil(t, storage)
		assert.Equal(t, tempDir, storage.BasePath)

		// Verify directory was created
		_, err = os.Stat(tempDir)
		assert.NoError(t, err)
	})

	t.Run("Create with empty path uses default", func(t *testing.T) {
		storage, err := NewBrandingFileStorage("")
		assert.NoError(t, err)
		assert.Equal(t, DefaultBrandingStoragePath, storage.BasePath)

		// Clean up
		os.RemoveAll(DefaultBrandingStoragePath)
	})

	t.Run("Create with nested path", func(t *testing.T) {
		tempDir := t.TempDir()
		nestedPath := filepath.Join(tempDir, "branding", "assets")

		storage, err := NewBrandingFileStorage(nestedPath)
		assert.NoError(t, err)
		assert.Equal(t, nestedPath, storage.BasePath)

		// Verify nested directory was created
		_, err = os.Stat(nestedPath)
		assert.NoError(t, err)
	})
}

func TestGetBrandingStoragePath(t *testing.T) {
	t.Run("Get default path when env not set", func(t *testing.T) {
		// Ensure env var is not set
		os.Unsetenv("BRANDING_STORAGE_PATH")

		path := GetBrandingStoragePath()
		assert.Equal(t, DefaultBrandingStoragePath, path)
	})

	t.Run("Get path from environment variable", func(t *testing.T) {
		customPath := "/custom/branding/path"
		os.Setenv("BRANDING_STORAGE_PATH", customPath)
		defer os.Unsetenv("BRANDING_STORAGE_PATH")

		path := GetBrandingStoragePath()
		assert.Equal(t, customPath, path)
	})
}

func TestSaveLogo(t *testing.T) {
	storage, _ := setupBrandingStorageTest(t)

	t.Run("Save valid PNG logo", func(t *testing.T) {
		// Create minimal PNG file data
		pngData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		}

		file := newMockFile(pngData)
		header := createMockFileHeader("logo.png", "image/png", int64(len(pngData)))

		filename, err := storage.SaveLogo(file, header)
		assert.NoError(t, err)
		assert.Equal(t, "logo.png", filename)

		// Verify file was saved
		assert.True(t, storage.FileExists(filename))
	})

	t.Run("Save valid JPEG logo", func(t *testing.T) {
		jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG signature

		file := newMockFile(jpegData)
		header := createMockFileHeader("logo.jpg", "image/jpeg", int64(len(jpegData)))

		filename, err := storage.SaveLogo(file, header)
		assert.NoError(t, err)
		assert.Equal(t, "logo.jpg", filename)
	})

	t.Run("Reject logo that's too large", func(t *testing.T) {
		largeData := make([]byte, MaxLogoSize+1)

		file := newMockFile(largeData)
		header := createMockFileHeader("large.png", "image/png", int64(len(largeData)))

		filename, err := storage.SaveLogo(file, header)
		assert.Error(t, err)
		assert.Equal(t, ErrFileTooLarge, err)
		assert.Empty(t, filename)
	})

	t.Run("Reject invalid logo file type", func(t *testing.T) {
		data := []byte("not an image")

		file := newMockFile(data)
		header := createMockFileHeader("logo.gif", "image/gif", int64(len(data)))

		filename, err := storage.SaveLogo(file, header)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidFileType, err)
		assert.Empty(t, filename)
	})

	t.Run("Replace existing logo", func(t *testing.T) {
		// Save first logo
		data1 := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		file1 := newMockFile(data1)
		header1 := createMockFileHeader("logo.png", "image/png", int64(len(data1)))
		filename1, err := storage.SaveLogo(file1, header1)
		assert.NoError(t, err)

		// Save second logo with different extension
		data2 := []byte{0xFF, 0xD8, 0xFF, 0xE0}
		file2 := newMockFile(data2)
		header2 := createMockFileHeader("logo.jpg", "image/jpeg", int64(len(data2)))
		filename2, err := storage.SaveLogo(file2, header2)
		assert.NoError(t, err)

		// Old logo should be deleted
		assert.False(t, storage.FileExists(filename1))
		// New logo should exist
		assert.True(t, storage.FileExists(filename2))
	})
}

func TestSaveFavicon(t *testing.T) {
	storage, _ := setupBrandingStorageTest(t)

	t.Run("Save valid ICO favicon", func(t *testing.T) {
		// Minimal ICO file data
		icoData := []byte{
			0x00, 0x00, 0x01, 0x00, // ICO signature
			0x01, 0x00, // 1 image
		}

		file := newMockFile(icoData)
		header := createMockFileHeader("favicon.ico", "image/x-icon", int64(len(icoData)))

		filename, err := storage.SaveFavicon(file, header)
		assert.NoError(t, err)
		assert.Equal(t, "favicon.ico", filename)
		assert.True(t, storage.FileExists(filename))
	})

	t.Run("Reject favicon that's too large", func(t *testing.T) {
		largeData := make([]byte, MaxFaviconSize+1)

		file := newMockFile(largeData)
		header := createMockFileHeader("favicon.ico", "image/x-icon", int64(len(largeData)))

		filename, err := storage.SaveFavicon(file, header)
		assert.Error(t, err)
		assert.Equal(t, ErrFileTooLarge, err)
		assert.Empty(t, filename)
	})

	t.Run("Reject invalid favicon type", func(t *testing.T) {
		data := []byte("not an icon")

		file := newMockFile(data)
		header := createMockFileHeader("favicon.bmp", "image/bmp", int64(len(data)))

		filename, err := storage.SaveFavicon(file, header)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidFileType, err)
		assert.Empty(t, filename)
	})
}

func TestDeleteLogo(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	t.Run("Delete existing logo", func(t *testing.T) {
		// Create a test logo file
		logoPath := filepath.Join(tempDir, "logo.png")
		err := os.WriteFile(logoPath, []byte("fake logo"), 0644)
		assert.NoError(t, err)

		err = storage.DeleteLogo("logo.png")
		assert.NoError(t, err)

		// Verify file was deleted
		assert.False(t, storage.FileExists("logo.png"))
	})

	t.Run("Delete non-existent logo doesn't error", func(t *testing.T) {
		err := storage.DeleteLogo("nonexistent.png")
		assert.NoError(t, err)
	})

	t.Run("Delete empty filename doesn't error", func(t *testing.T) {
		err := storage.DeleteLogo("")
		assert.NoError(t, err)
	})
}

func TestDeleteFavicon(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	t.Run("Delete existing favicon", func(t *testing.T) {
		// Create a test favicon file
		faviconPath := filepath.Join(tempDir, "favicon.ico")
		err := os.WriteFile(faviconPath, []byte("fake favicon"), 0644)
		assert.NoError(t, err)

		err = storage.DeleteFavicon("favicon.ico")
		assert.NoError(t, err)

		// Verify file was deleted
		assert.False(t, storage.FileExists("favicon.ico"))
	})

	t.Run("Delete non-existent favicon doesn't error", func(t *testing.T) {
		err := storage.DeleteFavicon("nonexistent.ico")
		assert.NoError(t, err)
	})

	t.Run("Delete empty filename doesn't error", func(t *testing.T) {
		err := storage.DeleteFavicon("")
		assert.NoError(t, err)
	})
}

func TestGetFilePath(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	t.Run("Get file path for filename", func(t *testing.T) {
		path := storage.GetFilePath("logo.png")
		expected := filepath.Join(tempDir, "logo.png")
		assert.Equal(t, expected, path)
	})

	t.Run("Get file path for empty filename", func(t *testing.T) {
		path := storage.GetFilePath("")
		assert.Equal(t, "", path)
	})
}

func TestFileExists(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	t.Run("File exists returns true for existing file", func(t *testing.T) {
		// Create a test file
		testFile := filepath.Join(tempDir, "test.png")
		err := os.WriteFile(testFile, []byte("test"), 0644)
		assert.NoError(t, err)

		exists := storage.FileExists("test.png")
		assert.True(t, exists)
	})

	t.Run("File exists returns false for non-existent file", func(t *testing.T) {
		exists := storage.FileExists("nonexistent.png")
		assert.False(t, exists)
	})

	t.Run("File exists returns false for empty filename", func(t *testing.T) {
		exists := storage.FileExists("")
		assert.False(t, exists)
	})
}

func TestCleanupOldFiles(t *testing.T) {
	storage, tempDir := setupBrandingStorageTest(t)

	t.Run("Cleanup removes matching files", func(t *testing.T) {
		// Create multiple logo files with different extensions
		os.WriteFile(filepath.Join(tempDir, "logo.png"), []byte("png"), 0644)
		os.WriteFile(filepath.Join(tempDir, "logo.jpg"), []byte("jpg"), 0644)
		os.WriteFile(filepath.Join(tempDir, "logo.svg"), []byte("svg"), 0644)
		os.WriteFile(filepath.Join(tempDir, "favicon.ico"), []byte("ico"), 0644)

		// Cleanup logo files
		err := storage.cleanupOldFiles("logo")
		assert.NoError(t, err)

		// All logo files should be removed
		assert.False(t, storage.FileExists("logo.png"))
		assert.False(t, storage.FileExists("logo.jpg"))
		assert.False(t, storage.FileExists("logo.svg"))

		// Favicon should still exist
		assert.True(t, storage.FileExists("favicon.ico"))
	})

	t.Run("Cleanup with no matching files doesn't error", func(t *testing.T) {
		err := storage.cleanupOldFiles("nonexistent")
		assert.NoError(t, err)
	})

	t.Run("Cleanup with non-existent directory doesn't error", func(t *testing.T) {
		storage2 := &BrandingFileStorage{
			BasePath: filepath.Join(tempDir, "nonexistent-dir"),
		}

		err := storage2.cleanupOldFiles("logo")
		assert.NoError(t, err)
	})
}
