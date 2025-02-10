package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/providers"
	"github.com/TykTechnologies/midsommar/v2/providers/direct"
	"github.com/TykTechnologies/midsommar/v2/providers/tyk"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/gin-gonic/gin"
)

type ProviderAPI struct {
	api *API
}

func NewProviderAPI(api *API) *ProviderAPI {
	// Initialize secrets package with database reference
	secrets.SetDBRef(api.service.DB)

	// Register direct import provider
	directProvider := direct.NewDirectProvider()
	api.providers.RegisterProvider("direct", directProvider)

	return &ProviderAPI{
		api: api,
	}
}

// @Summary List available OpenAPI providers
// @Description Get a list of all registered OpenAPI specification providers
// @Tags providers
// @Produce json
// @Success 200 {array} providers.ImportMethod
// @Router /providers [get]
func (a *ProviderAPI) listProviders(c *gin.Context) {
	methods := []providers.ImportMethod{
		{
			Type:        "provider",
			Name:        "Tyk Dashboard",
			Description: "Import API specifications from your Tyk Dashboard",
			Provider:    "tyk",
			NeedsConfig: true,
		},
		{
			Type:        "provider",
			Name:        "Direct Import",
			Description: "Import OpenAPI specifications directly via URL or file upload",
			Provider:    "direct",
			NeedsConfig: false,
		},
	}
	c.JSON(http.StatusOK, gin.H{"data": methods})
}

// ConfigureProviderRequest represents the request body for configuring a provider
type ConfigureProviderRequest struct {
	Config providers.ProviderConfig `json:"config"`
}

// @Summary Configure provider credentials
// @Description Set up credentials for a specific OpenAPI provider
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param config body ConfigureProviderRequest true "Provider configuration"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /providers/{id}/configure [post]
func (a *ProviderAPI) configureProvider(c *gin.Context) {
	id := c.Param("id")

	var req ConfigureProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	_, err := a.api.providers.GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
		return
	}

	// Create a new provider instance with the provided config
	var newProvider providers.OpenAPIProvider
	switch id {
	case "tyk":
		// Resolve any secret references in the token
		config := req.Config
		config.Token = secrets.GetValue(config.Token)
		newProvider = tyk.NewTykDashboardProvider(config)
	case "direct":
		// Direct provider doesn't need configuration
		c.JSON(http.StatusOK, gin.H{"data": "Direct provider doesn't require configuration"})
		return
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Unsupported provider"}},
		})
		return
	}

	// Validate the credentials
	if err := newProvider.ValidateCredentials(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid credentials: " + err.Error()}},
		})
		return
	}

	// Update the provider in the registry
	if err := a.api.providers.RemoveProvider(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	if err := a.api.providers.RegisterProvider(id, newProvider); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "Provider configured successfully"})
}

// @Summary Get API specifications from provider
// @Description Retrieve available API specifications from a specific provider
// @Tags providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {array} providers.APISpec
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/specs [get]
func (a *ProviderAPI) getProviderSpecs(c *gin.Context) {
	id := c.Param("id")

	provider, err := a.api.providers.GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
		return
	}

	switch id {
	case "tyk":
		// For Tyk provider, check if it's configured
		tykProvider, ok := provider.(*tyk.TykDashboardProvider)
		if !ok {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Invalid provider type"}},
			})
			return
		}
		if tykProvider.Config.URL == "" || tykProvider.Config.Token == "" {
			c.JSON(http.StatusOK, gin.H{"data": []providers.APISpec{}})
			return
		}
	case "direct":
		// Direct provider doesn't need configuration checks
		break
	}

	specs, err := provider.GetAPISpecs()
	if err != nil {
		// Return empty array instead of error for unconfigured provider
		if err.Error() == "error making request: Get \"/api/apis\": unsupported protocol scheme \"\"" {
			c.JSON(http.StatusOK, gin.H{"data": []providers.APISpec{}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": specs})
}

// ImportRequest represents the unified request body for importing a spec
type ImportRequest struct {
	Method string `json:"method" binding:"required"` // "url" or "file"
	URL    string `json:"url"`                       // Required if method is "url"
	Name   string `json:"name" binding:"required"`
}

// @Summary Get import steps
// @Description Get the steps required for importing an OpenAPI specification
// @Tags providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {object} providers.ImportStep
// @Router /providers/{id}/import-steps [get]
func (a *ProviderAPI) getImportSteps(c *gin.Context) {
	id := c.Param("id")

	provider, err := a.api.providers.GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
		return
	}

	switch id {
	case "direct":
		// Direct import shows URL/file options immediately
		step := providers.ImportStep{
			Type:        "import_method",
			Provider:    "direct",
			CurrentStep: 1,
			TotalSteps:  1, // Just the import method step
			Methods: []providers.ImportMethod{
				{
					Type:        "url",
					Name:        "Import from URL",
					Description: "Import an OpenAPI specification from a URL",
					Provider:    "direct",
					NeedsConfig: false,
				},
				{
					Type:        "file",
					Name:        "Upload File",
					Description: "Import an OpenAPI specification from a file",
					Provider:    "direct",
					NeedsConfig: false,
				},
			},
		}
		c.JSON(http.StatusOK, gin.H{"data": step})
	case "tyk":
		// For Tyk provider, check if it's configured
		tykProvider, ok := provider.(*tyk.TykDashboardProvider)
		if !ok {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Invalid provider type"}},
			})
			return
		}

		if tykProvider.Config.URL == "" || tykProvider.Config.Token == "" {
			// Not configured, show config step
			step := providers.ImportStep{
				Type:        "config",
				Provider:    "tyk",
				CurrentStep: 1,
				TotalSteps:  2, // Config + select API
				Methods: []providers.ImportMethod{
					{
						Type:        "provider",
						Name:        "Tyk Dashboard",
						Description: "Import from your Tyk Dashboard",
						Provider:    "tyk",
						NeedsConfig: true,
					},
				},
			}
			c.JSON(http.StatusOK, gin.H{"data": step})
		} else {
			// Already configured, show API selection
			step := providers.ImportStep{
				Type:        "select_api",
				Provider:    "tyk",
				CurrentStep: 2,
				TotalSteps:  2,                          // Config + select API
				Methods:     []providers.ImportMethod{}, // No methods needed for API selection
			}
			c.JSON(http.StatusOK, gin.H{"data": step})
		}
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Unsupported provider"}},
		})
	}
}

// @Summary Import OpenAPI spec
// @Description Import an OpenAPI specification using the specified method
// @Tags providers
// @Accept multipart/form-data,json
// @Produce json
// @Param request body ImportRequest true "Import request"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Router /providers/direct/import [post]
func (a *ProviderAPI) importSpec(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")

	var method, name, url string
	var fileContent []byte

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle form data (file upload)
		method = c.PostForm("method")
		name = c.PostForm("name")
		url = c.PostForm("url")

		if method == "file" {
			file, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Bad Request", Detail: "File upload required for file method"}},
				})
				return
			}

			f, err := file.Open()
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Internal Server Error", Detail: err.Error()}},
				})
				return
			}
			defer f.Close()

			fileContent, err = io.ReadAll(f)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Internal Server Error", Detail: err.Error()}},
				})
				return
			}
		}
	} else {
		// Handle JSON request
		var req ImportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
			})
			return
		}
		method = req.Method
		name = req.Name
		url = req.URL
	}

	if method == "" || name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Method and name are required"}},
		})
		return
	}

	provider, err := a.api.providers.GetProvider("direct")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	directProvider, ok := provider.(*direct.DirectProvider)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Invalid provider type"}},
		})
		return
	}

	switch method {
	case "url":
		if url == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "URL is required for URL method"}},
			})
			return
		}
		if err := directProvider.ImportFromURL(url, name); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
			})
			return
		}
	case "file":
		if fileContent == nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "File content is required for file method"}},
			})
			return
		}
		if err := directProvider.ImportFromFile(fileContent, name); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
			})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid import method"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "OpenAPI spec imported successfully"})
}

func (a *ProviderAPI) RegisterRoutes(router *gin.RouterGroup) {
	providers := router.Group("/providers")
	{
		providers.GET("", a.listProviders)
		providers.POST("/:id/configure", a.configureProvider)
		providers.GET("/:id/specs", a.getProviderSpecs)

		// Import endpoints
		providers.GET("/:id/import-steps", a.getImportSteps)
		providers.POST("/direct/import", a.importSpec)
	}
}
