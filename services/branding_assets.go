package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The logo and favicon are stored in the database (models.BrandingAsset),
// so every replica serves them, whichever one received the upload. They
// used to be files under BRANDING_STORAGE_PATH on that one replica: fine
// for replicas sharing a volume, not for two sites. The directory is now
// read once, at start, to import assets uploaded before (ImportBrandingFiles).

// brandingAssetKind describes one kind of asset.
type brandingAssetKind struct {
	kind    string
	maxSize int64
	// allowed maps an accepted upload content type to its file extension.
	allowed map[string]string
	// served maps a file extension to the content type it is served with.
	served map[string]string
}

var (
	brandingLogo = brandingAssetKind{
		kind: models.BrandingAssetLogo, maxSize: MaxLogoSize, allowed: AllowedLogoTypes,
		served: map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".svg": "image/svg+xml"},
	}
	brandingFavicon = brandingAssetKind{
		kind: models.BrandingAssetFavicon, maxSize: MaxFaviconSize, allowed: AllowedFaviconTypes,
		served: map[string]string{".ico": "image/x-icon", ".png": "image/png"},
	}
)

// brandingCacheTTL is how long a replica serves its in-memory copy of an
// asset before checking the stored hash again. Uploads and resets reach the
// other replicas at once by replicas.SignalBranding; this covers a signal
// that never arrived.
const brandingCacheTTL = 30 * time.Second

// brandingAssetCache holds this replica's copies of the stored assets, so
// serving the logo does not read megabytes from the database each time.
type brandingAssetCache struct {
	mu      sync.Mutex
	entries map[string]brandingCacheEntry
}

type brandingCacheEntry struct {
	asset   *models.BrandingAsset // nil: no asset stored
	checked time.Time
}

func (c *brandingAssetCache) get(kind string) (brandingCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[kind]
	return e, ok
}

func (c *brandingAssetCache) put(kind string, asset *models.BrandingAsset) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]brandingCacheEntry{}
	}
	c.entries[kind] = brandingCacheEntry{asset: asset, checked: time.Now()}
}

func (c *brandingAssetCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// InvalidateBrandingAssets drops this replica's in-memory copies of the logo
// and favicon; the next request reads them from the database. It runs when
// another replica signals replicas.SignalBranding.
func (s *Service) InvalidateBrandingAssets() {
	s.brandingAssets.clear()
}

// brandingAssetsChanged drops this replica's copies and tells the others.
func (s *Service) brandingAssetsChanged() {
	s.brandingAssets.clear()
	replicas.Signal(context.Background(), replicas.SignalBranding)
}

// BrandingLogo returns the uploaded logo, or nil when there is none.
func (s *Service) BrandingLogo() (*models.BrandingAsset, error) {
	return s.brandingAsset(models.BrandingAssetLogo)
}

// BrandingFavicon returns the uploaded favicon, or nil when there is none.
func (s *Service) BrandingFavicon() (*models.BrandingAsset, error) {
	return s.brandingAsset(models.BrandingAssetFavicon)
}

// brandingAsset returns the stored asset of kind from this replica's copy,
// checking the stored hash once the copy is brandingCacheTTL old. When the
// database cannot be read, a copy already held is served as it is.
func (s *Service) brandingAsset(kind string) (*models.BrandingAsset, error) {
	cached, ok := s.brandingAssets.get(kind)
	if ok && time.Since(cached.checked) < brandingCacheTTL {
		return cached.asset, nil
	}
	if ok {
		hash, err := models.BrandingAssetHash(s.DB, kind)
		if err != nil {
			logger.Debugf("Branding %s: serving the copy in memory, the database could not be read: %v", kind, err)
			return cached.asset, nil
		}
		held := ""
		if cached.asset != nil {
			held = cached.asset.Hash
		}
		if hash == held {
			s.brandingAssets.put(kind, cached.asset)
			return cached.asset, nil
		}
	}
	asset, err := models.GetBrandingAsset(s.DB, kind)
	if err != nil {
		if ok {
			logger.Debugf("Branding %s: serving the copy in memory, the database could not be read: %v", kind, err)
			return cached.asset, nil
		}
		return nil, err
	}
	s.brandingAssets.put(kind, asset)
	return asset, nil
}

// readBrandingUpload validates an upload as the file storage did (declared
// size and content type) and returns its bytes, the name recorded in the
// settings ("logo.png") and the content type it is served with. The bytes
// are read with the limit applied, whatever size the header declares.
func readBrandingUpload(k brandingAssetKind, file multipart.File, header *multipart.FileHeader) ([]byte, string, string, error) {
	if header.Size > k.maxSize {
		return nil, "", "", ErrFileTooLarge
	}
	extension, ok := k.allowed[header.Header.Get("Content-Type")]
	if !ok {
		return nil, "", "", ErrInvalidFileType
	}
	data, err := io.ReadAll(io.LimitReader(file, k.maxSize+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read file: %w", err)
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("failed to read file: %w", io.EOF)
	}
	if int64(len(data)) > k.maxSize {
		return nil, "", "", ErrFileTooLarge
	}
	return data, k.kind + extension, k.served[extension], nil
}

func newBrandingAsset(kind, contentType string, data []byte) *models.BrandingAsset {
	sum := sha256.Sum256(data)
	return &models.BrandingAsset{
		Kind: kind, ContentType: contentType, Data: data,
		Hash: hex.EncodeToString(sum[:]), Size: int64(len(data)),
	}
}

// uploadBrandingAsset stores an uploaded asset and records its name in the
// settings, in one transaction.
func (s *Service) uploadBrandingAsset(k brandingAssetKind, file multipart.File, header *multipart.FileHeader, what string) (*models.BrandingSettings, error) {
	data, name, contentType, err := readBrandingUpload(k, file, header)
	if err != nil {
		if errors.Is(err, ErrFileTooLarge) || errors.Is(err, ErrInvalidFileType) {
			return nil, err // the API matches these by identity
		}
		return nil, fmt.Errorf("failed to save %s: %w", what, err)
	}
	settings := &models.BrandingSettings{}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := models.SaveBrandingAsset(tx, newBrandingAsset(k.kind, contentType, data)); err != nil {
			return fmt.Errorf("failed to save %s: %w", what, err)
		}
		if err := settings.Get(tx); err != nil {
			return err
		}
		if k.kind == models.BrandingAssetLogo {
			settings.LogoPath = name
		} else {
			settings.FaviconPath = name
		}
		if err := settings.Update(tx); err != nil {
			return fmt.Errorf("failed to update %s path: %w", what, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.brandingAssetsChanged()
	return settings, nil
}

// resetBranding removes the stored assets and resets the settings, in one
// transaction. Files from before assets were stored in the database are
// left alone: with the settings reset they are never imported again.
func (s *Service) resetBranding() (*models.BrandingSettings, error) {
	settings := &models.BrandingSettings{}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := models.DeleteBrandingAssets(tx); err != nil {
			return fmt.Errorf("failed to delete branding assets: %w", err)
		}
		if err := settings.ResetToDefaults(tx); err != nil {
			return fmt.Errorf("failed to reset settings: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.brandingAssetsChanged()
	return settings, nil
}

// ImportBrandingFiles copies a logo or favicon uploaded before assets were
// stored in the database from basePath (BRANDING_STORAGE_PATH) into it. An
// asset is imported when the settings name a custom one, the database holds
// none, and its file is on this replica's disk; so it happens once, on the
// first replica that has the file, and never after a reset. The files are
// left in place. It returns the kinds imported. Problems with one asset are
// logged and do not stop the other, or start-up.
func (s *Service) ImportBrandingFiles(basePath string) []string {
	settings, err := s.GetBrandingSettings()
	if err != nil {
		logger.Warnf("Branding: could not read the settings to import assets from %s: %v", basePath, err)
		return nil
	}
	var imported []string
	for _, c := range []struct {
		k      brandingAssetKind
		custom bool
		name   string
	}{
		{brandingLogo, settings.HasCustomLogo(), settings.LogoPath},
		{brandingFavicon, settings.HasCustomFavicon(), settings.FaviconPath},
	} {
		if !c.custom {
			continue
		}
		ok, err := s.importBrandingFile(c.k, basePath, c.name)
		switch {
		case err != nil:
			logger.Warnf("Branding: the %s was not imported from %s: %v", c.k.kind, basePath, err)
		case ok:
			logger.Infof("Branding: imported the %s from %s into the database; the file is no longer needed", c.k.kind, filepath.Join(basePath, filepath.Base(c.name)))
			imported = append(imported, c.k.kind)
		}
	}
	if len(imported) > 0 {
		s.brandingAssetsChanged()
	}
	return imported
}

func (s *Service) importBrandingFile(k brandingAssetKind, basePath, name string) (bool, error) {
	hash, err := models.BrandingAssetHash(s.DB, k.kind)
	if err != nil {
		return false, err
	}
	if hash != "" {
		return false, nil // already in the database
	}
	if basePath == "" {
		basePath = DefaultBrandingStoragePath
	}
	// The settings hold a bare file name ("logo.png"); never follow one out
	// of the directory.
	name = filepath.Base(name)
	contentType, ok := k.served[strings.ToLower(filepath.Ext(name))]
	if !ok {
		return false, fmt.Errorf("%q is not a %s file type", name, k.kind)
	}
	f, err := os.Open(filepath.Join(basePath, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil // uploaded to another replica, or already cleaned up
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, k.maxSize+1))
	if err != nil {
		return false, err
	}
	if len(data) == 0 || int64(len(data)) > k.maxSize {
		return false, fmt.Errorf("%s is empty or larger than %d bytes", name, k.maxSize)
	}
	return models.AddBrandingAsset(s.DB, newBrandingAsset(k.kind, contentType, data))
}
