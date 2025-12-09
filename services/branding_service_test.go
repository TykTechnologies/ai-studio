package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBrandingTest(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)
	return service, db
}

func TestGetBrandingSettings(t *testing.T) {
	service, _ := setupBrandingTest(t)

	// First call should create default settings
	settings, err := service.GetBrandingSettings()
	assert.NoError(t, err)
	assert.NotNil(t, settings)

	// Should have default values
	assert.NotEmpty(t, settings.AppTitle)
	assert.NotEmpty(t, settings.PrimaryColor)
	assert.NotEmpty(t, settings.SecondaryColor)
	assert.NotEmpty(t, settings.BackgroundColor)

	// Second call should return same settings
	settings2, err := service.GetBrandingSettings()
	assert.NoError(t, err)
	assert.Equal(t, settings.ID, settings2.ID)
}

func TestUpdateBrandingSettings_Validation(t *testing.T) {
	service, _ := setupBrandingTest(t)

	tests := []struct {
		name        string
		isAdmin     bool
		setup       func(*models.BrandingSettings)
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Non-admin cannot update",
			isAdmin:     false,
			setup:       func(s *models.BrandingSettings) { s.AppTitle = "New Title" },
			expectError: true,
			errorMsg:    "only admin",
		},
		{
			name:    "Admin can update valid settings",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.AppTitle = "Valid Title"
				s.PrimaryColor = "#FF0000"
			},
			expectError: false,
		},
		{
			name:    "Invalid primary color rejected",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.PrimaryColor = "not-a-color"
			},
			expectError: true,
			errorMsg:    "invalid",
		},
		{
			name:    "Invalid secondary color rejected",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.SecondaryColor = "#GGGGGG"
			},
			expectError: true,
			errorMsg:    "invalid",
		},
		{
			name:    "Invalid background color rejected",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.BackgroundColor = "rgb(255,0,0)"
			},
			expectError: true,
			errorMsg:    "invalid",
		},
		{
			name:    "Title too long rejected",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.AppTitle = "This is a very long title that exceeds the fifty character limit and should be rejected"
			},
			expectError: true,
			errorMsg:    "exceeds maximum",
		},
		{
			name:    "Empty colors are allowed",
			isAdmin: true,
			setup: func(s *models.BrandingSettings) {
				s.PrimaryColor = ""
				s.AppTitle = "Empty Colors OK"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := service.GetBrandingSettings()
			assert.NoError(t, err)

			tt.setup(settings)

			result, err := service.UpdateBrandingSettings(settings, tt.isAdmin)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestUpdateBrandingColors(t *testing.T) {
	service, _ := setupBrandingTest(t)

	t.Run("Admin can update colors", func(t *testing.T) {
		result, err := service.UpdateBrandingColors("#FF0000", "#00FF00", "#0000FF", true)
		assert.NoError(t, err)
		assert.Equal(t, "#FF0000", result.PrimaryColor)
		assert.Equal(t, "#00FF00", result.SecondaryColor)
		assert.Equal(t, "#0000FF", result.BackgroundColor)
	})

	t.Run("Non-admin cannot update colors", func(t *testing.T) {
		result, err := service.UpdateBrandingColors("#FFFFFF", "", "", false)
		assert.Error(t, err)
		assert.Equal(t, ErrUnauthorized, err)
		assert.Nil(t, result)
	})

	t.Run("Invalid colors rejected", func(t *testing.T) {
		result, err := service.UpdateBrandingColors("invalid", "", "", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
		assert.Nil(t, result)
	})

	t.Run("Partial updates work", func(t *testing.T) {
		// Update only primary color
		result, err := service.UpdateBrandingColors("#AABBCC", "", "", true)
		assert.NoError(t, err)
		assert.Equal(t, "#AABBCC", result.PrimaryColor)
	})
}

func TestUpdateAppTitle(t *testing.T) {
	service, _ := setupBrandingTest(t)

	t.Run("Admin can update title", func(t *testing.T) {
		result, err := service.UpdateAppTitle("New App Title", true)
		assert.NoError(t, err)
		assert.Equal(t, "New App Title", result.AppTitle)
	})

	t.Run("Non-admin cannot update title", func(t *testing.T) {
		result, err := service.UpdateAppTitle("Unauthorized Title", false)
		assert.Error(t, err)
		assert.Equal(t, ErrUnauthorized, err)
		assert.Nil(t, result)
	})

	t.Run("Title too long rejected", func(t *testing.T) {
		longTitle := "This is a very long title that exceeds the fifty character limit"
		result, err := service.UpdateAppTitle(longTitle, true)
		assert.Error(t, err)
		assert.Equal(t, ErrAppTitleTooLong, err)
		assert.Nil(t, result)
	})

	t.Run("Title at limit accepted", func(t *testing.T) {
		exactLimit := "12345678901234567890123456789012345678901234567890" // Exactly 50
		result, err := service.UpdateAppTitle(exactLimit, true)
		assert.NoError(t, err)
		assert.Equal(t, exactLimit, result.AppTitle)
	})
}

func TestUpdateCustomCSS(t *testing.T) {
	service, _ := setupBrandingTest(t)

	t.Run("Admin can update CSS", func(t *testing.T) {
		css := ".custom { color: red; }"
		result, err := service.UpdateCustomCSS(css, true)
		assert.NoError(t, err)
		assert.Equal(t, css, result.CustomCSS)
	})

	t.Run("Non-admin cannot update CSS", func(t *testing.T) {
		result, err := service.UpdateCustomCSS(".hacker {}", false)
		assert.Error(t, err)
		assert.Equal(t, ErrUnauthorized, err)
		assert.Nil(t, result)
	})

	t.Run("Empty CSS allowed", func(t *testing.T) {
		result, err := service.UpdateCustomCSS("", true)
		assert.NoError(t, err)
		assert.Equal(t, "", result.CustomCSS)
	})
}

func TestGetLogoFilePath(t *testing.T) {
	service, _ := setupBrandingTest(t)

	t.Run("Returns empty when no custom logo", func(t *testing.T) {
		path, err := service.GetLogoFilePath()
		assert.NoError(t, err)
		assert.Empty(t, path)
	})

	t.Run("Returns path when custom logo exists", func(t *testing.T) {
		// Set a logo path
		settings, err := service.GetBrandingSettings()
		assert.NoError(t, err)
		settings.LogoPath = "test-logo.png"
		err = settings.Update(service.DB)
		assert.NoError(t, err)

		path, err := service.GetLogoFilePath()
		assert.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "test-logo.png")
	})
}

func TestGetFaviconFilePath(t *testing.T) {
	service, _ := setupBrandingTest(t)

	t.Run("Returns empty when no custom favicon", func(t *testing.T) {
		path, err := service.GetFaviconFilePath()
		assert.NoError(t, err)
		assert.Empty(t, path)
	})

	t.Run("Returns path when custom favicon exists", func(t *testing.T) {
		// Set a favicon path
		settings, err := service.GetBrandingSettings()
		assert.NoError(t, err)
		settings.FaviconPath = "test-favicon.ico"
		err = settings.Update(service.DB)
		assert.NoError(t, err)

		path, err := service.GetFaviconFilePath()
		assert.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "test-favicon.ico")
	})
}

// Note: UploadLogo, UploadFavicon, and ResetBrandingToDefaults require file system operations
// and are better tested through integration tests or API layer tests
// They are already covered by api/branding_handlers_test.go
