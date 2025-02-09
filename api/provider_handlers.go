package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/providers"
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
	return &ProviderAPI{
		api: api,
	}
}

// @Summary List available OpenAPI providers
// @Description Get a list of all registered OpenAPI specification providers
// @Tags providers
// @Produce json
// @Success 200 {array} providers.Provider
// @Router /providers [get]
func (a *ProviderAPI) listProviders(c *gin.Context) {
	providers := a.api.providers.ListProviders()
	c.JSON(http.StatusOK, gin.H{"data": providers})
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

	// For Tyk provider, check if it's configured
	if id == "tyk" {
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

// RegisterRoutes registers the provider API routes
func (a *ProviderAPI) RegisterRoutes(router *gin.RouterGroup) {
	providers := router.Group("/providers")
	{
		providers.GET("", a.listProviders)
		providers.POST("/:id/configure", a.configureProvider)
		providers.GET("/:id/specs", a.getProviderSpecs)
	}
}
