package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
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
	_ = user // identity is checked; authorization is the branding:write check

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
	settings, err = a.service.UpdateBrandingSettings(settings, authz.Can(c, authz.Write("branding")))
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
	_ = user // identity is checked; authorization is the branding:write check

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
	settings, err := a.service.UploadLogo(file, header, authz.Can(c, authz.Write("branding")))
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
			// The cause (a database error) goes to the log, not the response.
			logger.Errorf("Branding: could not save the logo: %v", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Failed to save the logo"}},
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
	_ = user // identity is checked; authorization is the branding:write check

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
	settings, err := a.service.UploadFavicon(file, header, authz.Can(c, authz.Write("branding")))
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
			// The cause (a database error) goes to the log, not the response.
			logger.Errorf("Branding: could not save the favicon: %v", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Failed to save the favicon"}},
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
	_ = user // identity is checked; authorization is the branding:write check

	// Reset to defaults
	settings, err := a.service.ResetBrandingToDefaults(authz.Can(c, authz.Write("branding")))
	if err != nil {
		if err == services.ErrUnauthorized {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "Only admin users can reset branding settings"}},
			})
		} else {
			// The cause (a database error) goes to the log, not the response.
			logger.Errorf("Branding: could not reset the branding settings: %v", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Failed to reset the branding settings"}},
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
	asset, err := a.service.BrandingLogo()
	a.serveBrandingAsset(c, asset, err, "logo", "logos/tyk-portal-logo.png", "image/png")
}

// serveBrandingAsset answers with the uploaded asset from the database, or
// the embedded default when there is none. The uploaded asset carries its
// hash as ETag and "Cache-Control: no-cache": browsers keep it and ask each
// time whether it changed (304 when it did not), so a new upload shows at
// once, from whichever replica answers.
func (a *API) serveBrandingAsset(c *gin.Context, asset *models.BrandingAsset, err error, what, defaultFile, defaultType string) {
	if err != nil {
		// The cause (a database error) goes to the log, not to the caller:
		// this endpoint is public.
		logger.Errorf("Branding: could not read the %s: %v", what, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get the " + what}},
		})
		return
	}

	// If no custom asset, serve the embedded default
	if asset == nil {
		data, err := fs.ReadFile(a.frontend, defaultFile)
		if err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Default " + what + " not found"}},
			})
			return
		}
		c.Data(http.StatusOK, defaultType, data)
		return
	}

	// An uploaded asset is served as what it was stored as, never sniffed,
	// and as an inert document: an SVG logo opened directly (rather than
	// shown in an <img>, where scripts never run) is sandboxed and may load
	// nothing, so a script inside it does not run on Studio's origin.
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")

	etag := `"` + asset.Hash + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "no-cache")
	if match := c.GetHeader("If-None-Match"); match != "" && etagMatches(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, asset.ContentType, asset.Data)
}

// etagMatches reports whether an If-None-Match header names etag (or any).
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			return true
		}
	}
	return false
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
	asset, err := a.service.BrandingFavicon()
	a.serveBrandingAsset(c, asset, err, "favicon", "sun.ico", "image/x-icon")
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
	Type       string                     `json:"type"`
	ID         string                     `json:"id"`
	Attributes BrandingSettingsAttributes `json:"attributes"`
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
