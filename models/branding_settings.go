package models

import (
	"gorm.io/gorm"
)

// Default branding values (current Tyk branding)
const (
	DefaultPrimaryColor     = "#23E2C2"
	DefaultSecondaryColor   = "#343452"
	DefaultBackgroundColor  = "#FFFFFF"
	DefaultAppTitle         = "Tyk AI Portal"
	DefaultLogoPath         = ""
	DefaultFaviconPath      = ""
	DefaultCustomCSS        = ""
	BrandingSettingsSingletonID = 1
)

// BrandingSettings stores system-wide UI customization settings
// Uses singleton pattern - only one record with ID=1 exists
type BrandingSettings struct {
	gorm.Model
	ID                uint   `json:"id" gorm:"primaryKey"`
	LogoPath          string `json:"logo_path"`          // Path to custom logo file (empty = use default)
	FaviconPath       string `json:"favicon_path"`       // Path to custom favicon file (empty = use default)
	AppTitle          string `json:"app_title"`          // Custom application title
	PrimaryColor      string `json:"primary_color"`      // Hex color for primary brand color
	SecondaryColor    string `json:"secondary_color"`    // Hex color for secondary brand color
	BackgroundColor   string `json:"background_color"`   // Hex color for background
	CustomCSS         string `json:"custom_css" gorm:"type:text"` // Custom CSS overrides
}

// NewBrandingSettings creates a new BrandingSettings instance with default values
func NewBrandingSettings() *BrandingSettings {
	return &BrandingSettings{
		ID:              BrandingSettingsSingletonID,
		LogoPath:        DefaultLogoPath,
		FaviconPath:     DefaultFaviconPath,
		AppTitle:        DefaultAppTitle,
		PrimaryColor:    DefaultPrimaryColor,
		SecondaryColor:  DefaultSecondaryColor,
		BackgroundColor: DefaultBackgroundColor,
		CustomCSS:       DefaultCustomCSS,
	}
}

// Get retrieves the singleton branding settings record
// Creates default settings if none exist
func (bs *BrandingSettings) Get(db *gorm.DB) error {
	err := db.First(bs, BrandingSettingsSingletonID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create default settings if none exist
			*bs = *NewBrandingSettings()
			return bs.Create(db)
		}
		return err
	}
	return nil
}

// Create creates the branding settings record
// Should only be called once during initialization
func (bs *BrandingSettings) Create(db *gorm.DB) error {
	bs.ID = BrandingSettingsSingletonID
	return db.Create(bs).Error
}

// Update updates the existing branding settings
func (bs *BrandingSettings) Update(db *gorm.DB) error {
	bs.ID = BrandingSettingsSingletonID
	return db.Save(bs).Error
}

// ResetToDefaults resets all settings to default values
func (bs *BrandingSettings) ResetToDefaults(db *gorm.DB) error {
	*bs = *NewBrandingSettings()
	return bs.Update(db)
}

// HasCustomLogo returns true if a custom logo has been set
func (bs *BrandingSettings) HasCustomLogo() bool {
	return bs.LogoPath != "" && bs.LogoPath != DefaultLogoPath
}

// HasCustomFavicon returns true if a custom favicon has been set
func (bs *BrandingSettings) HasCustomFavicon() bool {
	return bs.FaviconPath != "" && bs.FaviconPath != DefaultFaviconPath
}

// ToFrontendConfig converts branding settings to frontend-compatible format
func (bs *BrandingSettings) ToFrontendConfig() map[string]interface{} {
	return map[string]interface{}{
		"app_title":         bs.AppTitle,
		"primary_color":     bs.PrimaryColor,
		"secondary_color":   bs.SecondaryColor,
		"background_color":  bs.BackgroundColor,
		"custom_css":        bs.CustomCSS,
		"has_custom_logo":   bs.HasCustomLogo(),
		"has_custom_favicon": bs.HasCustomFavicon(),
	}
}
