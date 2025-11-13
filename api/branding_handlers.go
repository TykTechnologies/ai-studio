package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// @Summary Get branding settings
// @Description Get current branding settings (public endpoint for frontend config)
// @Tags branding
// @Accept json
// @Produce json
// @Success 200 {object} BrandingSettingsResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/settings [get]
func (a *API) getBrandingSettings(c *gin.Context) {
	settings, err := a.service.GetBrandingSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get branding settings: " + err.Error()}},
		})
		return
	}

	response := BrandingSettingsResponse{
		Type: "branding_settings",
		ID:   "1",
		Attributes: BrandingSettingsAttributes{
			AppTitle:          settings.AppTitle,
			PrimaryColor:      settings.PrimaryColor,
			SecondaryColor:    settings.SecondaryColor,
			BackgroundColor:   settings.BackgroundColor,
			CustomCSS:         settings.CustomCSS,
			HasCustomLogo:     settings.HasCustomLogo(),
			HasCustomFavicon:  settings.HasCustomFavicon(),
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Update branding settings
// @Description Update branding settings (admin only)
// @Tags branding
// @Accept json
// @Produce json
// @Param settings body UpdateBrandingSettingsRequest true "Branding settings"
// @Success 200 {object} BrandingSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/settings [put]
// @Security BearerAuth
func (a *API) updateBrandingSettings(c *gin.Context) {
	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found in context"}},
		})
		return
	}
	currentUser := user.(*models.User)

	// Parse request
	var req UpdateBrandingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// Get current settings
	settings, err := a.service.GetBrandingSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get current settings: " + err.Error()}},
		})
		return
	}

	// Update fields if provided
	if req.AppTitle != nil {
		settings.AppTitle = *req.AppTitle
	}
	if req.PrimaryColor != nil {
		settings.PrimaryColor = *req.PrimaryColor
	}
	if req.SecondaryColor != nil {
		settings.SecondaryColor = *req.SecondaryColor
	}
	if req.BackgroundColor != nil {
		settings.BackgroundColor = *req.BackgroundColor
	}
	if req.CustomCSS != nil {
		settings.CustomCSS = *req.CustomCSS
	}

	// Update settings
	settings, err = a.service.UpdateBrandingSettings(settings, currentUser.IsAdmin)
	if err != nil {
		if err == services.ErrUnauthorized {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "Only admin users can update branding settings"}},
			})
		} else {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
			})
		}
		return
	}

	response := BrandingSettingsResponse{
		Type: "branding_settings",
		ID:   "1",
		Attributes: BrandingSettingsAttributes{
			AppTitle:         settings.AppTitle,
			PrimaryColor:     settings.PrimaryColor,
			SecondaryColor:   settings.SecondaryColor,
			BackgroundColor:  settings.BackgroundColor,
			CustomCSS:        settings.CustomCSS,
			HasCustomLogo:    settings.HasCustomLogo(),
			HasCustomFavicon: settings.HasCustomFavicon(),
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Upload logo
// @Description Upload a custom logo (admin only, PNG/JPG/SVG, max 2MB)
// @Tags branding
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Logo file"
// @Success 200 {object} BrandingSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/logo [post]
// @Security BearerAuth
func (a *API) uploadLogo(c *gin.Context) {
	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found in context"}},
		})
		return
	}
	currentUser := user.(*models.User)

	// Get file from request
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Error getting file from request: " + err.Error()}},
		})
		return
	}
	defer file.Close()

	// Upload logo
	settings, err := a.service.UploadLogo(file, header, currentUser.IsAdmin)
	if err != nil {
		if err == services.ErrUnauthorized {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "Only admin users can upload logos"}},
			})
		} else if err == services.ErrFileTooLarge {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "File size exceeds maximum limit (2MB)"}},
			})
		} else if err == services.ErrInvalidFileType {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid file type. Allowed types: PNG, JPG, SVG"}},
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
		}
		return
	}

	response := BrandingSettingsResponse{
		Type: "branding_settings",
		ID:   "1",
		Attributes: BrandingSettingsAttributes{
			AppTitle:         settings.AppTitle,
			PrimaryColor:     settings.PrimaryColor,
			SecondaryColor:   settings.SecondaryColor,
			BackgroundColor:  settings.BackgroundColor,
			CustomCSS:        settings.CustomCSS,
			HasCustomLogo:    settings.HasCustomLogo(),
			HasCustomFavicon: settings.HasCustomFavicon(),
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Upload favicon
// @Description Upload a custom favicon (admin only, ICO/PNG, max 100KB)
// @Tags branding
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Favicon file"
// @Success 200 {object} BrandingSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/favicon [post]
// @Security BearerAuth
func (a *API) uploadFavicon(c *gin.Context) {
	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found in context"}},
		})
		return
	}
	currentUser := user.(*models.User)

	// Get file from request
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Error getting file from request: " + err.Error()}},
		})
		return
	}
	defer file.Close()

	// Upload favicon
	settings, err := a.service.UploadFavicon(file, header, currentUser.IsAdmin)
	if err != nil {
		if err == services.ErrUnauthorized {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "Only admin users can upload favicons"}},
			})
		} else if err == services.ErrFileTooLarge {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "File size exceeds maximum limit (100KB)"}},
			})
		} else if err == services.ErrInvalidFileType {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid file type. Allowed types: ICO, PNG"}},
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
		}
		return
	}

	response := BrandingSettingsResponse{
		Type: "branding_settings",
		ID:   "1",
		Attributes: BrandingSettingsAttributes{
			AppTitle:         settings.AppTitle,
			PrimaryColor:     settings.PrimaryColor,
			SecondaryColor:   settings.SecondaryColor,
			BackgroundColor:  settings.BackgroundColor,
			CustomCSS:        settings.CustomCSS,
			HasCustomLogo:    settings.HasCustomLogo(),
			HasCustomFavicon: settings.HasCustomFavicon(),
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Reset branding to defaults
// @Description Reset all branding settings to default values (admin only)
// @Tags branding
// @Accept json
// @Produce json
// @Success 200 {object} BrandingSettingsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/reset [post]
// @Security BearerAuth
func (a *API) resetBranding(c *gin.Context) {
	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found in context"}},
		})
		return
	}
	currentUser := user.(*models.User)

	// Reset to defaults
	settings, err := a.service.ResetBrandingToDefaults(currentUser.IsAdmin)
	if err != nil {
		if err == services.ErrUnauthorized {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "Only admin users can reset branding settings"}},
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
		}
		return
	}

	response := BrandingSettingsResponse{
		Type: "branding_settings",
		ID:   "1",
		Attributes: BrandingSettingsAttributes{
			AppTitle:         settings.AppTitle,
			PrimaryColor:     settings.PrimaryColor,
			SecondaryColor:   settings.SecondaryColor,
			BackgroundColor:  settings.BackgroundColor,
			CustomCSS:        settings.CustomCSS,
			HasCustomLogo:    settings.HasCustomLogo(),
			HasCustomFavicon: settings.HasCustomFavicon(),
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get custom logo
// @Description Serve the custom logo file (public endpoint)
// @Tags branding
// @Produce image/png,image/jpeg,image/svg+xml
// @Success 200 {file} file
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/logo [get]
func (a *API) serveLogo(c *gin.Context) {
	logoPath, err := a.service.GetLogoFilePath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get logo path: " + err.Error()}},
		})
		return
	}

	// If no custom logo, serve embedded default
	if logoPath == "" {
		// Serve embedded logo from static files
		data, err := a.staticFiles.ReadFile("ui/admin-frontend/build/logos/tyk-portal-logo.png")
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Default logo not found"}},
			})
			return
		}
		c.Data(http.StatusOK, "image/png", data)
		return
	}

	// Serve custom logo
	if _, err := os.Stat(logoPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Logo file not found"}},
		})
		return
	}

	// Determine content type based on file extension
	ext := filepath.Ext(logoPath)
	contentType := "application/octet-stream"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".svg":
		contentType = "image/svg+xml"
	}

	c.File(logoPath)
	c.Header("Content-Type", contentType)
}

// @Summary Get custom favicon
// @Description Serve the custom favicon file (public endpoint)
// @Tags branding
// @Produce image/x-icon,image/png
// @Success 200 {file} file
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/branding/favicon [get]
func (a *API) serveFavicon(c *gin.Context) {
	faviconPath, err := a.service.GetFaviconFilePath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get favicon path: " + err.Error()}},
		})
		return
	}

	// If no custom favicon, serve embedded default
	if faviconPath == "" {
		// Serve embedded favicon from static files
		data, err := a.staticFiles.ReadFile("ui/admin-frontend/build/sun.ico")
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Default favicon not found"}},
			})
			return
		}
		c.Data(http.StatusOK, "image/x-icon", data)
		return
	}

	// Serve custom favicon
	if _, err := os.Stat(faviconPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Favicon file not found"}},
		})
		return
	}

	// Determine content type based on file extension
	ext := filepath.Ext(faviconPath)
	contentType := "application/octet-stream"
	switch ext {
	case ".ico":
		contentType = "image/x-icon"
	case ".png":
		contentType = "image/png"
	}

	c.File(faviconPath)
	c.Header("Content-Type", contentType)
}

// Request/Response models

type UpdateBrandingSettingsRequest struct {
	AppTitle        *string `json:"app_title"`
	PrimaryColor    *string `json:"primary_color"`
	SecondaryColor  *string `json:"secondary_color"`
	BackgroundColor *string `json:"background_color"`
	CustomCSS       *string `json:"custom_css"`
}

type BrandingSettingsResponse struct {
	Type       string                      `json:"type"`
	ID         string                      `json:"id"`
	Attributes BrandingSettingsAttributes  `json:"attributes"`
}

type BrandingSettingsAttributes struct {
	AppTitle         string `json:"app_title"`
	PrimaryColor     string `json:"primary_color"`
	SecondaryColor   string `json:"secondary_color"`
	BackgroundColor  string `json:"background_color"`
	CustomCSS        string `json:"custom_css"`
	HasCustomLogo    bool   `json:"has_custom_logo"`
	HasCustomFavicon bool   `json:"has_custom_favicon"`
}
