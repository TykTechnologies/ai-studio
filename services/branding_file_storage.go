package services

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Default branding storage path
	DefaultBrandingStoragePath = "./data/branding"

	// File size limits
	MaxLogoSize    = 2 * 1024 * 1024  // 2MB
	MaxFaviconSize = 100 * 1024        // 100KB

	// Allowed file extensions
	LogoFileName    = "logo"
	FaviconFileName = "favicon"
)

var (
	// Allowed MIME types and extensions for logos
	AllowedLogoTypes = map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/svg+xml": ".svg",
	}

	// Allowed MIME types and extensions for favicons
	AllowedFaviconTypes = map[string]string{
		"image/x-icon":      ".ico",
		"image/vnd.microsoft.icon": ".ico",
		"image/png":         ".png",
	}

	ErrFileTooLarge        = errors.New("file size exceeds maximum allowed")
	ErrInvalidFileType     = errors.New("invalid file type")
	ErrStoragePathNotSet   = errors.New("branding storage path not configured")
)

// BrandingFileStorage handles file operations for branding assets
type BrandingFileStorage struct {
	BasePath string
}

// NewBrandingFileStorage creates a new branding file storage instance
func NewBrandingFileStorage(basePath string) (*BrandingFileStorage, error) {
	if basePath == "" {
		basePath = DefaultBrandingStoragePath
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create branding storage directory: %w", err)
	}

	return &BrandingFileStorage{
		BasePath: basePath,
	}, nil
}

// GetBrandingStoragePath returns the configured branding storage path from environment or default
func GetBrandingStoragePath() string {
	path := os.Getenv("BRANDING_STORAGE_PATH")
	if path == "" {
		return DefaultBrandingStoragePath
	}
	return path
}

// SaveLogo saves an uploaded logo file
func (bfs *BrandingFileStorage) SaveLogo(file multipart.File, header *multipart.FileHeader) (string, error) {
	// Validate file size
	if header.Size > MaxLogoSize {
		return "", ErrFileTooLarge
	}

	// Detect content type
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("failed to reset file pointer: %w", err)
	}

	contentType := header.Header.Get("Content-Type")
	extension, ok := AllowedLogoTypes[contentType]
	if !ok {
		return "", ErrInvalidFileType
	}

	// Clean up old logo files
	if err := bfs.cleanupOldFiles(LogoFileName); err != nil {
		return "", fmt.Errorf("failed to cleanup old logo files: %w", err)
	}

	// Generate filename
	filename := LogoFileName + extension
	filepath := filepath.Join(bfs.BasePath, filename)

	// Save file
	if err := bfs.saveFile(file, filepath); err != nil {
		return "", err
	}

	return filename, nil
}

// SaveFavicon saves an uploaded favicon file
func (bfs *BrandingFileStorage) SaveFavicon(file multipart.File, header *multipart.FileHeader) (string, error) {
	// Validate file size
	if header.Size > MaxFaviconSize {
		return "", ErrFileTooLarge
	}

	// Detect content type
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("failed to reset file pointer: %w", err)
	}

	contentType := header.Header.Get("Content-Type")
	extension, ok := AllowedFaviconTypes[contentType]
	if !ok {
		return "", ErrInvalidFileType
	}

	// Clean up old favicon files
	if err := bfs.cleanupOldFiles(FaviconFileName); err != nil {
		return "", fmt.Errorf("failed to cleanup old favicon files: %w", err)
	}

	// Generate filename
	filename := FaviconFileName + extension
	filepath := filepath.Join(bfs.BasePath, filename)

	// Save file
	if err := bfs.saveFile(file, filepath); err != nil {
		return "", err
	}

	return filename, nil
}

// saveFile writes the uploaded file to disk
func (bfs *BrandingFileStorage) saveFile(file multipart.File, filepath string) error {
	dst, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

// cleanupOldFiles removes old logo/favicon files with different extensions
func (bfs *BrandingFileStorage) cleanupOldFiles(baseName string) error {
	entries, err := os.ReadDir(bfs.BasePath)
	if err != nil {
		// Directory doesn't exist yet, nothing to clean up
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if file starts with the base name (logo or favicon)
		filename := entry.Name()
		if strings.HasPrefix(filename, baseName+".") || filename == baseName {
			fullPath := filepath.Join(bfs.BasePath, filename)
			if err := os.Remove(fullPath); err != nil {
				return fmt.Errorf("failed to remove old file %s: %w", filename, err)
			}
		}
	}

	return nil
}

// DeleteLogo deletes the current logo file
func (bfs *BrandingFileStorage) DeleteLogo(filename string) error {
	if filename == "" {
		return nil
	}

	filepath := filepath.Join(bfs.BasePath, filename)
	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete logo: %w", err)
	}

	return nil
}

// DeleteFavicon deletes the current favicon file
func (bfs *BrandingFileStorage) DeleteFavicon(filename string) error {
	if filename == "" {
		return nil
	}

	filepath := filepath.Join(bfs.BasePath, filename)
	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete favicon: %w", err)
	}

	return nil
}

// GetFilePath returns the full path to a branding file
func (bfs *BrandingFileStorage) GetFilePath(filename string) string {
	if filename == "" {
		return ""
	}
	return filepath.Join(bfs.BasePath, filename)
}

// FileExists checks if a file exists in the branding storage
func (bfs *BrandingFileStorage) FileExists(filename string) bool {
	if filename == "" {
		return false
	}

	filepath := bfs.GetFilePath(filename)
	_, err := os.Stat(filepath)
	return err == nil
}
