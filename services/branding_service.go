package services

import (
	"errors"
	"fmt"
	"mime/multipart"
	"regexp"

	"github.com/TykTechnologies/midsommar/v2/models"
)

var (
	// Hex color validation regex
	hexColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

	ErrInvalidColor     = errors.New("invalid hex color format (expected #RRGGBB)")
	ErrAppTitleTooLong  = errors.New("app title exceeds maximum length (50 characters)")
	ErrUnauthorized     = errors.New("only admin users can modify branding settings")
)

// GetBrandingSettings retrieves the current branding settings
// Creates default settings if none exist
func (s *Service) GetBrandingSettings() (*models.BrandingSettings, error) {
	settings := &models.BrandingSettings{}
	if err := settings.Get(s.DB); err != nil {
		return nil, fmt.Errorf("failed to get branding settings: %w", err)
	}
	return settings, nil
}

// UpdateBrandingSettings updates branding settings with validation
func (s *Service) UpdateBrandingSettings(settings *models.BrandingSettings, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}

	// Validate settings
	if err := s.validateBrandingSettings(settings); err != nil {
		return nil, err
	}

	// Update settings
	if err := settings.Update(s.DB); err != nil {
		return nil, fmt.Errorf("failed to update branding settings: %w", err)
	}

	return settings, nil
}

// UpdateBrandingColors updates only the color settings
func (s *Service) UpdateBrandingColors(primaryColor, secondaryColor, backgroundColor string, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}

	// Validate colors
	if primaryColor != "" && !hexColorRegex.MatchString(primaryColor) {
		return nil, fmt.Errorf("invalid primary color: %w", ErrInvalidColor)
	}
	if secondaryColor != "" && !hexColorRegex.MatchString(secondaryColor) {
		return nil, fmt.Errorf("invalid secondary color: %w", ErrInvalidColor)
	}
	if backgroundColor != "" && !hexColorRegex.MatchString(backgroundColor) {
		return nil, fmt.Errorf("invalid background color: %w", ErrInvalidColor)
	}

	// Get current settings
	settings, err := s.GetBrandingSettings()
	if err != nil {
		return nil, err
	}

	// Update colors
	if primaryColor != "" {
		settings.PrimaryColor = primaryColor
	}
	if secondaryColor != "" {
		settings.SecondaryColor = secondaryColor
	}
	if backgroundColor != "" {
		settings.BackgroundColor = backgroundColor
	}

	// Save settings
	if err := settings.Update(s.DB); err != nil {
		return nil, fmt.Errorf("failed to update colors: %w", err)
	}

	return settings, nil
}

// UpdateAppTitle updates the application title
func (s *Service) UpdateAppTitle(title string, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}

	if len(title) > 50 {
		return nil, ErrAppTitleTooLong
	}

	// Get current settings
	settings, err := s.GetBrandingSettings()
	if err != nil {
		return nil, err
	}

	settings.AppTitle = title

	// Save settings
	if err := settings.Update(s.DB); err != nil {
		return nil, fmt.Errorf("failed to update app title: %w", err)
	}

	return settings, nil
}

// UpdateCustomCSS updates the custom CSS
func (s *Service) UpdateCustomCSS(css string, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}

	// Get current settings
	settings, err := s.GetBrandingSettings()
	if err != nil {
		return nil, err
	}

	settings.CustomCSS = css

	// Save settings
	if err := settings.Update(s.DB); err != nil {
		return nil, fmt.Errorf("failed to update custom CSS: %w", err)
	}

	return settings, nil
}

// UploadLogo handles logo file upload. The logo is stored in the database
// (see branding_assets.go).
func (s *Service) UploadLogo(file multipart.File, header *multipart.FileHeader, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}
	return s.uploadBrandingAsset(brandingLogo, file, header, "logo")
}

// UploadFavicon handles favicon file upload. The favicon is stored in the
// database (see branding_assets.go).
func (s *Service) UploadFavicon(file multipart.File, header *multipart.FileHeader, isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}
	return s.uploadBrandingAsset(brandingFavicon, file, header, "favicon")
}

// ResetBrandingToDefaults resets all branding settings to defaults and
// removes the uploaded logo and favicon.
func (s *Service) ResetBrandingToDefaults(isAdmin bool) (*models.BrandingSettings, error) {
	if !isAdmin {
		return nil, ErrUnauthorized
	}
	return s.resetBranding()
}

// GetLogoFilePath returns the full file path of a logo uploaded before
// assets were stored in the database (what ImportBrandingFiles reads).
func (s *Service) GetLogoFilePath() (string, error) {
	settings, err := s.GetBrandingSettings()
	if err != nil {
		return "", err
	}

	if !settings.HasCustomLogo() {
		return "", nil
	}

	storage, err := NewBrandingFileStorage(GetBrandingStoragePath())
	if err != nil {
		return "", err
	}

	return storage.GetFilePath(settings.LogoPath), nil
}

// GetFaviconFilePath returns the full file path of a favicon uploaded
// before assets were stored in the database.
func (s *Service) GetFaviconFilePath() (string, error) {
	settings, err := s.GetBrandingSettings()
	if err != nil {
		return "", err
	}

	if !settings.HasCustomFavicon() {
		return "", nil
	}

	storage, err := NewBrandingFileStorage(GetBrandingStoragePath())
	if err != nil {
		return "", err
	}

	return storage.GetFilePath(settings.FaviconPath), nil
}

// validateBrandingSettings validates all branding settings fields
func (s *Service) validateBrandingSettings(settings *models.BrandingSettings) error {
	// Validate colors
	if settings.PrimaryColor != "" && !hexColorRegex.MatchString(settings.PrimaryColor) {
		return fmt.Errorf("invalid primary color: %w", ErrInvalidColor)
	}
	if settings.SecondaryColor != "" && !hexColorRegex.MatchString(settings.SecondaryColor) {
		return fmt.Errorf("invalid secondary color: %w", ErrInvalidColor)
	}
	if settings.BackgroundColor != "" && !hexColorRegex.MatchString(settings.BackgroundColor) {
		return fmt.Errorf("invalid background color: %w", ErrInvalidColor)
	}

	// Validate app title length
	if len(settings.AppTitle) > 50 {
		return ErrAppTitleTooLong
	}

	return nil
}
