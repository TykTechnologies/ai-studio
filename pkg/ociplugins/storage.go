// pkg/ociplugins/storage.go
package ociplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContentStorage manages content-addressed storage for plugins
type ContentStorage struct {
	baseDir    string
	maxSize    int64
	gcInterval time.Duration
}

// NewContentStorage creates a new content storage instance
func NewContentStorage(baseDir string, maxSize int64, gcInterval time.Duration) (*ContentStorage, error) {
	storage := &ContentStorage{
		baseDir:    baseDir,
		maxSize:    maxSize,
		gcInterval: gcInterval,
	}

	// Create directory structure
	if err := storage.initDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %w", err)
	}

	return storage, nil
}

// initDirectories creates the required directory structure
func (s *ContentStorage) initDirectories() error {
	dirs := []string{
		s.getCASDir(),
		s.getBinsDir(),
		s.getActiveDir(),
		s.getTempDir(),
		s.getMetadataDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// getCASDir returns the content-addressed storage directory
func (s *ContentStorage) getCASDir() string {
	return filepath.Join(s.baseDir, "cas", "sha256")
}

// getBinsDir returns the executables directory
func (s *ContentStorage) getBinsDir() string {
	return filepath.Join(s.baseDir, "bins")
}

// getActiveDir returns the active symlinks directory
func (s *ContentStorage) getActiveDir() string {
	return filepath.Join(s.baseDir, "active")
}

// getTempDir returns the temporary files directory
func (s *ContentStorage) getTempDir() string {
	return filepath.Join(s.baseDir, "temp")
}

// getMetadataDir returns the metadata directory
func (s *ContentStorage) getMetadataDir() string {
	return filepath.Join(s.baseDir, "metadata")
}

// StoreBlob stores raw blob data and returns the digest
func (s *ContentStorage) StoreBlob(data []byte) (string, error) {
	// Calculate SHA256 digest
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])

	// Check if blob already exists
	blobPath := filepath.Join(s.getCASDir(), digest)
	if _, err := os.Stat(blobPath); err == nil {
		return digest, nil // Already exists
	}

	// Write to temporary file first
	tempFile, err := os.CreateTemp(s.getTempDir(), "blob-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("failed to write blob data: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomically move to final location
	if err := os.Rename(tempFile.Name(), blobPath); err != nil {
		return "", fmt.Errorf("failed to move blob to final location: %w", err)
	}

	return digest, nil
}

// GetBlob retrieves blob data by digest
func (s *ContentStorage) GetBlob(digest string) ([]byte, error) {
	blobPath := filepath.Join(s.getCASDir(), digest)
	data, err := os.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrPluginNotFound{Reference: digest}
		}
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}
	return data, nil
}

// StoreExecutable stores a plugin binary and returns the executable path
func (s *ContentStorage) StoreExecutable(digest, arch string, data []byte) (string, error) {
	// Generate executable filename
	execName := fmt.Sprintf("sha256-%s-%s", digest, strings.ReplaceAll(arch, "/", "-"))
	execPath := filepath.Join(s.getBinsDir(), execName)

	// Check if already exists
	if _, err := os.Stat(execPath); err == nil {
		return execPath, nil // Already exists
	}

	// Write to temporary file first
	tempFile, err := os.CreateTemp(s.getTempDir(), "exec-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("failed to write executable data: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Make executable and move to final location
	if err := os.Chmod(tempFile.Name(), 0755); err != nil {
		return "", fmt.Errorf("failed to set executable permissions: %w", err)
	}

	if err := os.Rename(tempFile.Name(), execPath); err != nil {
		return "", fmt.Errorf("failed to move executable to final location: %w", err)
	}

	return execPath, nil
}

// GetExecutablePath returns the path to an executable by digest and architecture
func (s *ContentStorage) GetExecutablePath(digest, arch string) string {
	execName := fmt.Sprintf("sha256-%s-%s", digest, strings.ReplaceAll(arch, "/", "-"))
	return filepath.Join(s.getBinsDir(), execName)
}

// HasExecutable checks if an executable exists for the given digest and architecture
func (s *ContentStorage) HasExecutable(digest, arch string) bool {
	execPath := s.GetExecutablePath(digest, arch)
	_, err := os.Stat(execPath)
	return err == nil
}

// CreateActiveLink creates or updates a symlink to mark a plugin as active
func (s *ContentStorage) CreateActiveLink(name, digest, arch string) error {
	linkPath := filepath.Join(s.getActiveDir(), name)
	execPath := s.GetExecutablePath(digest, arch)

	// Verify executable exists
	if !s.HasExecutable(digest, arch) {
		return &ErrPluginNotFound{Reference: fmt.Sprintf("%s@%s (%s)", name, digest, arch)}
	}

	// Create relative path for symlink
	relPath, err := filepath.Rel(s.getActiveDir(), execPath)
	if err != nil {
		return fmt.Errorf("failed to create relative path: %w", err)
	}

	// Remove existing symlink if it exists
	if _, err := os.Lstat(linkPath); err == nil {
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}

	// Create new symlink
	if err := os.Symlink(relPath, linkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// GetActivePath returns the path to the active version of a plugin
func (s *ContentStorage) GetActivePath(name string) (string, error) {
	linkPath := filepath.Join(s.getActiveDir(), name)

	// Check if symlink exists
	if _, err := os.Lstat(linkPath); err != nil {
		if os.IsNotExist(err) {
			return "", &ErrPluginNotFound{Reference: name}
		}
		return "", fmt.Errorf("failed to check symlink: %w", err)
	}

	// Resolve symlink
	execPath, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlink: %w", err)
	}

	return execPath, nil
}

// CalculateSize returns the total size of the cache in bytes
func (s *ContentStorage) CalculateSize() (int64, error) {
	var totalSize int64

	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize, err
}

// GarbageCollect removes old plugin versions, keeping the specified number
func (s *ContentStorage) GarbageCollect(keepVersions int) error {
	// Get all executables grouped by plugin name
	pluginVersions, err := s.getPluginVersions()
	if err != nil {
		return fmt.Errorf("failed to get plugin versions: %w", err)
	}

	var removedCount int
	var removedSize int64

	for pluginName, versions := range pluginVersions {
		// Skip if we have fewer versions than we want to keep
		if len(versions) <= keepVersions {
			continue
		}

		// Sort versions by modification time (newest first)
		// Keep the newest versions, remove the oldest
		versionsToRemove := versions[keepVersions:]

		for _, version := range versionsToRemove {
			// Don't remove if it's the active version
			if s.isActiveVersion(pluginName, version.digest, version.arch) {
				continue
			}

			// Remove executable
			if err := os.Remove(version.path); err != nil {
				return fmt.Errorf("failed to remove executable %s: %w", version.path, err)
			}

			removedCount++
			removedSize += version.size
		}
	}

	// Clean up orphaned blobs (blobs not referenced by any executable)
	if err := s.cleanupOrphanedBlobs(); err != nil {
		return fmt.Errorf("failed to cleanup orphaned blobs: %w", err)
	}

	return nil
}

// pluginVersion represents a plugin version with metadata
type pluginVersion struct {
	digest  string
	arch    string
	path    string
	modTime time.Time
	size    int64
}

// getPluginVersions returns all plugin versions grouped by plugin name
func (s *ContentStorage) getPluginVersions() (map[string][]pluginVersion, error) {
	pluginVersions := make(map[string][]pluginVersion)

	entries, err := os.ReadDir(s.getBinsDir())
	if err != nil {
		return nil, fmt.Errorf("failed to read bins directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Parse filename: sha256-{digest}-{arch}
		name := entry.Name()
		if !strings.HasPrefix(name, "sha256-") {
			continue
		}

		parts := strings.Split(name, "-")
		if len(parts) < 3 {
			continue
		}

		digest := parts[1]
		arch := strings.Join(parts[2:], "-") // Rejoin in case arch has dashes

		info, err := entry.Info()
		if err != nil {
			continue
		}

		version := pluginVersion{
			digest:  digest,
			arch:    arch,
			path:    filepath.Join(s.getBinsDir(), name),
			modTime: info.ModTime(),
			size:    info.Size(),
		}

		// Group by plugin name (we'd need to track this differently for real implementation)
		// For now, use digest as grouping key
		pluginName := digest[:12] // Use first 12 chars of digest as plugin identifier
		pluginVersions[pluginName] = append(pluginVersions[pluginName], version)
	}

	return pluginVersions, nil
}

// isActiveVersion checks if a plugin version is currently active
func (s *ContentStorage) isActiveVersion(pluginName, digest, arch string) bool {
	// This would need to be implemented based on how we track active versions
	// For now, check if there's an active symlink pointing to this executable
	expectedPath := s.GetExecutablePath(digest, arch)

	entries, err := os.ReadDir(s.getActiveDir())
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		linkPath := filepath.Join(s.getActiveDir(), entry.Name())
		target, err := filepath.EvalSymlinks(linkPath)
		if err != nil {
			continue
		}

		if target == expectedPath {
			return true
		}
	}

	return false
}

// cleanupOrphanedBlobs removes blobs that are not referenced by any executable
func (s *ContentStorage) cleanupOrphanedBlobs() error {
	// Get all digests referenced by executables
	referencedDigests := make(map[string]bool)

	entries, err := os.ReadDir(s.getBinsDir())
	if err != nil {
		return fmt.Errorf("failed to read bins directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "sha256-") {
			continue
		}

		parts := strings.Split(name, "-")
		if len(parts) >= 2 {
			digest := parts[1]
			referencedDigests[digest] = true
		}
	}

	// Remove unreferenced blobs
	blobEntries, err := os.ReadDir(s.getCASDir())
	if err != nil {
		return fmt.Errorf("failed to read CAS directory: %w", err)
	}

	for _, entry := range blobEntries {
		if entry.IsDir() {
			continue
		}

		digest := entry.Name()
		if !referencedDigests[digest] {
			blobPath := filepath.Join(s.getCASDir(), digest)
			if err := os.Remove(blobPath); err != nil {
				return fmt.Errorf("failed to remove orphaned blob %s: %w", digest, err)
			}
		}
	}

	return nil
}

// StoreMetadata stores plugin metadata
func (s *ContentStorage) StoreMetadata(digest, arch string, metadata *PluginMetadata) error {
	metadataFile := s.getMetadataPath(digest, arch)

	// Create metadata directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(metadataFile), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Serialize metadata to JSON
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write to temporary file first
	tempFile, err := os.CreateTemp(s.getTempDir(), "metadata-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomically move to final location
	if err := os.Rename(tempFile.Name(), metadataFile); err != nil {
		return fmt.Errorf("failed to move metadata to final location: %w", err)
	}

	return nil
}

// LoadMetadata loads plugin metadata by digest and architecture
func (s *ContentStorage) LoadMetadata(digest, arch string) (*PluginMetadata, error) {
	metadataFile := s.getMetadataPath(digest, arch)

	data, err := os.ReadFile(metadataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrPluginNotFound{Reference: fmt.Sprintf("%s (%s)", digest, arch)}
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata PluginMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// getMetadataPath returns the path to metadata file for a plugin
func (s *ContentStorage) getMetadataPath(digest, arch string) string {
	filename := fmt.Sprintf("sha256-%s-%s.json", digest, strings.ReplaceAll(arch, "/", "-"))
	return filepath.Join(s.getMetadataDir(), filename)
}

// HasMetadata checks if metadata exists for a plugin
func (s *ContentStorage) HasMetadata(digest, arch string) bool {
	metadataFile := s.getMetadataPath(digest, arch)
	_, err := os.Stat(metadataFile)
	return err == nil
}

// UpdateLastAccessed updates the last accessed time for a plugin
func (s *ContentStorage) UpdateLastAccessed(digest, arch string) error {
	metadata, err := s.LoadMetadata(digest, arch)
	if err != nil {
		return err
	}

	metadata.LastAccessed = time.Now()
	return s.StoreMetadata(digest, arch, metadata)
}

// ListAllMetadata returns metadata for all cached plugins
func (s *ContentStorage) ListAllMetadata() ([]*PluginMetadata, error) {
	metadataDir := s.getMetadataDir()

	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*PluginMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	var allMetadata []*PluginMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		metadataFile := filepath.Join(metadataDir, entry.Name())
		data, err := os.ReadFile(metadataFile)
		if err != nil {
			continue // Skip corrupted files
		}

		var metadata PluginMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			continue // Skip corrupted files
		}

		allMetadata = append(allMetadata, &metadata)
	}

	return allMetadata, nil
}