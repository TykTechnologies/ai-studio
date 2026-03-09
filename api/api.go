package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/providers"
	"github.com/TykTechnologies/midsommar/v2/providers/tyk"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/services/marketplace_management"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

// bodyLogWriter captures the response body while still writing it to the client
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// Write implements the io.Writer interface
func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// @title           Midsommar API
// @version         1.0
// @description     This is the API for the Midsommar user and group management system.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

type API struct {
	service                       *services.Service
	router                        *gin.Engine
	server                        *http.Server
	config                        *auth.Config
	disableCORS                   bool
	auth                          *auth.AuthService
	proxy                         *proxy.Proxy
	staticFiles                   embed.FS
	providers                     *providers.Registry
	setupChatRoutesFunc           func(*gin.RouterGroup)
	ssoService                    sso.Service
	licensingService              licensing.Service
	pluginSecurityService         plugin_security.Service
	marketplaceManagementService  marketplace_management.Service
	rateLimitCancel               context.CancelFunc
}

func NewAPI(service *services.Service, disableCORS bool, authService *auth.AuthService, config *auth.Config, proxy *proxy.Proxy, staticFiles embed.FS, licensingService licensing.Service) *API {
	gin.SetMode(gin.ReleaseMode)

	// Initialize provider registry
	providerRegistry := providers.NewRegistry()

	// Register the Tyk Dashboard provider by default
	tykProvider := tyk.NewTykDashboardProvider(providers.ProviderConfig{})
	if err := providerRegistry.RegisterProvider("tyk", tykProvider); err != nil {
		log.Printf("Failed to register Tyk provider: %v", err)
	}

	// Use gin.New() instead of gin.Default() to have control over middleware
	// gin.Default() adds Logger and Recovery middleware automatically
	router := gin.New()

	// Always add recovery middleware (handles panics)
	router.Use(gin.Recovery())

	// Only add Gin's request logger if debug logging is enabled
	if logger.IsDebugEnabled() {
		router.Use(gin.Logger())
	}

	// Add debug middleware only if DEBUG_HTTP=true
	if os.Getenv("DEBUG_HTTP") == "true" {
		router.Use(func(c *gin.Context) {
			// Log request details
			fmt.Printf("\n[DEBUG] %v | %v | Headers: %v\n", c.Request.Method, c.Request.URL.Path, c.Request.Header)

			// If there's a request body, read and log it
			if c.Request.Body != nil {
				bodyBytes, _ := io.ReadAll(c.Request.Body)
				// Restore the body for subsequent middleware/handlers
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// Try to pretty print if it's JSON
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err == nil {
					fmt.Printf("[DEBUG] Request Body:\n%s\n", prettyJSON.String())
				} else {
					fmt.Printf("[DEBUG] Request Body: %s\n", string(bodyBytes))
				}
			}

			// Get the response body
			blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw

			c.Next()

			// Log response status and body
			fmt.Printf("[DEBUG] Response Status: %d\n", c.Writer.Status())

			// Try to pretty print if it's JSON
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, blw.body.Bytes(), "", "  "); err == nil {
				fmt.Printf("[DEBUG] Response Body:\n%s\n", prettyJSON.String())
			} else {
				fmt.Printf("[DEBUG] Response Body: %s\n", blw.body.String())
			}
		})
	}

	api := &API{
		service:          service,
		router:           router,
		disableCORS:      disableCORS,
		auth:             authService,
		config:           config,
		proxy:            proxy,
		staticFiles:      staticFiles,
		providers:        providerRegistry,
		licensingService: licensingService,
	}

	// Add telemetry middleware (ENT: tracks API actions, CE: no-op)
	if licensingService != nil {
		router.Use(licensingService.TelemetryMiddleware())
	}

	// Initialize SSO service (ENT: full TIB functionality, CE: stub returning enterprise errors)
	logLevel := "info"
	if config.TestMode {
		logLevel = "debug"
	}

	ssoConfig := &sso.Config{
		APISecret: config.TIBAPISecret,
		LogLevel:  logLevel,
	}
	api.ssoService = sso.NewService(ssoConfig, router, config.DB, service.NotificationService)
	if sso.IsEnterpriseAvailable() {
		if err := api.ssoService.InitInternalTIB(); err != nil {
			log.Fatalf("Failed to initialize SSO service: %v", err)
		}
	}

	// Initialize Plugin Security service (ENT: full security enforcement, CE: stub allowing all operations)
	var ociLibConfig *ociplugins.OCIConfig
	if config.OCIConfig != nil {
		// Convert config.OCIConfig (interface{}) to *ociplugins.OCIConfig
		// This is set in main.go from appConf.OCIPlugins.ToOCILibConfig()
		if ociCfg, ok := config.OCIConfig.(*ociplugins.OCIConfig); ok {
			ociLibConfig = ociCfg
		}
	}

	pluginSecurityConfig := &plugin_security.Config{
		OCIConfig:                  ociLibConfig,
		AllowInternalNetworkAccess: os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") == "true",
	}
	api.pluginSecurityService = plugin_security.NewService(pluginSecurityConfig)

	// Initialize Marketplace Management service (ENT: full CRUD, CE: enterprise-only errors)
	api.marketplaceManagementService = marketplace_management.NewService(config.DB)

	api.setupChatRoutesFunc = api.SetupChatRoutes

	// Generate a random 32-byte key for CSRF
	csrfKey := make([]byte, 32)
	_, err := rand.Read(csrfKey)
	if err != nil {
		log.Fatalf("Failed to generate CSRF key: %v", err)
	}

	// no CSRF for tests
	if !config.TestMode {
		// Add CSRF middleware
		csrfMiddleware := csrf.Protect(
			csrfKey,
			csrf.Secure(false), // Allow HTTP in development
			csrf.Path("/"),
		)

		api.router.Use(func(c *gin.Context) {
			// Skip API calls
			if c.GetHeader("Authorization") != "" {
				c.Next()
				return
			}

			// Skip OAuth endpoints - they don't need CSRF protection
			if strings.HasPrefix(c.Request.URL.Path, "/oauth/") {
				c.Next()
				return
			}

			csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.Request = r
				c.Next()
			})).ServeHTTP(c.Writer, c.Request)
		})
	}

	api.setupRoutes()
	return api
}

func (a *API) Run(addr string, certFile string, keyFile string) error {
	// Create http.Server for graceful shutdown support
	a.server = &http.Server{
		Addr:    addr,
		Handler: a.router,
	}

	if certFile != "" && keyFile != "" {
		return a.server.ListenAndServeTLS(certFile, keyFile)
	}

	return a.server.ListenAndServe()
}

// Shutdown gracefully shuts down the API server
func (a *API) Shutdown(ctx context.Context) error {
	if a.rateLimitCancel != nil {
		a.rateLimitCancel()
	}

	if a.server == nil {
		return nil
	}

	logger.Info("Shutting down API server...")

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("API server shutdown failed: %w", err)
	}

	logger.Info("API server stopped successfully")
	return nil
}

// Helper function to create a sub-filesystem
func sub(fsys embed.FS, dir string) http.FileSystem {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// getPaginationParams extracts pagination parameters from the request
// If no parameters are provided, it returns default values for "all" pagination
func getPaginationParams(c *gin.Context) (int, int, bool) {
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}

	pageNumber, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || pageNumber <= 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string "json:\"title\""
				Detail string "json:\"detail\""
			}{{Title: "Bad Request", Detail: "Invalid page number"}},
		})
		return 0, 0, false
	}

	allStr := c.DefaultQuery("all", "false")
	all := strings.ToLower(allStr) == "true"

	return pageSize, pageNumber, all
}

func (a *API) setupRoutes() {
	// Add global panic recovery middleware
	a.router.Use(gin.Recovery())

	if a.disableCORS {
		a.router.Use(a.devCorsMiddleware())
	} else {
		a.router.Use(a.corsMiddleware())
	}

	a.router.GET("/sun.ico", func(c *gin.Context) {
		faviconFile, err := a.staticFiles.ReadFile("ui/admin-frontend/build/sun.ico")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/x-icon", faviconFile)
	})

	a.router.GET("/sun-logo.png", func(c *gin.Context) {
		faviconFile, err := a.staticFiles.ReadFile("ui/admin-frontend/build/sun-logo.png")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/png", faviconFile)
	})

	a.router.GET("/generic-datasource-icon.png", func(c *gin.Context) {
		faviconFile, err := a.staticFiles.ReadFile("ui/admin-frontend/build/generic-datasource-icon.png")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/png", faviconFile)
	})

	a.router.GET("/generic-llm-logo.png", func(c *gin.Context) {
		faviconFile, err := a.staticFiles.ReadFile("ui/admin-frontend/build/generic-llm-logo.png")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "image/png", faviconFile)
	})

	// Serve static files from /build/static
	staticFS, err := fs.Sub(a.staticFiles, "ui/admin-frontend/build/static")
	if err != nil {
		log.Fatal(err)
	}
	a.router.StaticFS("/static", http.FS(staticFS))

	// Serve logos from /build/logos
	logosFS, err := fs.Sub(a.staticFiles, "ui/admin-frontend/build/logos")
	if err != nil {
		log.Fatal(err)
	}
	a.router.StaticFS("/logos", http.FS(logosFS))

	// Set up rate limiters if enabled
	var rlEntries map[string]*rateLimitEntry
	if config.Get("").RateLimitEnabled {
		var rlCtx context.Context
		rlCtx, a.rateLimitCancel = context.WithCancel(context.Background())
		rlEntries = setupRateLimiters(rlCtx)
	}

	// OAuth 2.0 Authorization Server Endpoints - must be registered before NoRoute
	public := a.router.Group("/")
	oauthGroup := public.Group("/oauth")
	{
		// Dynamic Client Registration - public endpoint for now (auth will be added later)
		oauthGroup.POST("/register_client", a.handleRegisterOAuthClient)
		oauthGroup.OPTIONS("/register_client", a.handleRegisterOAuthClient)
		// Authorization Endpoint - requires user authentication (user logs in to grant access)
		// This endpoint will now redirect to a consent page if needed.
		oauthGroup.GET("/authorize", a.auth.AuthMiddleware(), a.handleOAuthAuthorize)
		oauthGroup.OPTIONS("/authorize", a.handleOAuthAuthorize)
		// Token Endpoint - typically requires client authentication
		if rlEntries != nil {
			oauthGroup.POST("/token", rateLimitHandler(rlEntries["oauth-token"]), a.handleOAuthToken)
		} else {
			oauthGroup.POST("/token", a.handleOAuthToken)
		}
		oauthGroup.OPTIONS("/token", a.handleOAuthToken)

		// Endpoints for consent screen flow - require user authentication
		oauthGroup.GET("/consent_details", a.auth.AuthMiddleware(), a.handleGetConsentDetails)
		oauthGroup.OPTIONS("/consent_details", a.handleGetConsentDetails)
		oauthGroup.POST("/submit_consent", a.auth.AuthMiddleware(), a.handleSubmitConsent)
		oauthGroup.OPTIONS("/submit_consent", a.handleSubmitConsent)
	}
	// AS Metadata - public
	public.GET("/.well-known/oauth-authorization-server", a.handleOAuthMetadata)
	public.OPTIONS("/.well-known/oauth-authorization-server", a.handleOAuthMetadata)

	// Serve index.html for all other routes, including /reset-password
	a.router.NoRoute(func(c *gin.Context) {
		// Check if it's a static file request
		if strings.HasPrefix(c.Request.URL.Path, "/static/") ||
			strings.HasPrefix(c.Request.URL.Path, "/logos/") ||
			strings.HasSuffix(c.Request.URL.Path, ".ico") ||
			strings.HasSuffix(c.Request.URL.Path, ".png") {
			c.Next()
			return
		}

		// Reject WebSocket/dev-server paths that shouldn't be handled by the SPA
		if c.Request.URL.Path == "/ws" {
			c.Status(http.StatusNotFound)
			return
		}

		// For all other routes, serve the frontend application
		indexFile, err := a.staticFiles.ReadFile("ui/admin-frontend/build/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Could not read index.html")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexFile)
	})

	a.router.GET("/csrf-token", func(c *gin.Context) {
		c.Header("X-CSRF-Token", csrf.Token(c.Request))
		c.Status(http.StatusOK)
	})

	// Analytics endpoints for portal users
	portalAnalytics := a.router.Group("/analytics")
	portalAnalytics.Use(a.auth.AuthMiddleware())
	portalAnalytics.GET("/token-usage-and-cost-for-app", a.getTokenUsageAndCostForApp)
	portalAnalytics.GET("/budget-usage-for-app", a.getBudgetUsageForApp)

	// Public routes
	if rlEntries != nil {
		public.POST("/auth/login", rateLimitHandler(rlEntries["login"]), a.handleLogin)
		public.POST("/auth/register", rateLimitHandler(rlEntries["register"]), a.handleRegister)
		public.POST("/auth/forgot-password", rateLimitHandler(rlEntries["forgot-password"]), a.handleForgotPassword)
		public.POST("/auth/resend-verification", rateLimitHandler(rlEntries["resend-verification"]), a.handleResendVerification)
	} else {
		public.POST("/auth/login", a.handleLogin)
		public.POST("/auth/register", a.handleRegister)
		public.POST("/auth/forgot-password", a.handleForgotPassword)
		public.POST("/auth/resend-verification", a.handleResendVerification)
	}
	public.POST("/auth/reset-password", a.handleResetPassword)
	public.GET("/auth/validate-reset-token", a.handleValidateResetToken)
	public.GET("/auth/verify-email", a.handleVerifyEmail)
	public.GET("/auth/config", a.handleGetConfig)
	public.GET("/auth/features", a.handleFeatureSet)

	// Health and readiness endpoints (unauthenticated)
	public.GET("/healthz", a.handleHealth)
	public.GET("/health", a.handleHealth)
	public.GET("/readyz", a.handleReadiness)
	public.GET("/ready", a.handleReadiness)

	// routes for portal users
	authed := public.Group("/common")
	authed.Use(a.auth.AuthMiddleware())
	authed.POST("/logout", a.handleLogout)
	authed.GET("/me", a.handleMe)
	authed.GET("/system", a.handleFeatureSet)

	// PORTAL FEATURES
	authed.GET("/catalogues/:id/llms", a.getCatalogueLLMs)
	authed.GET("/apps", a.getUserApps)
	authed.POST("/apps", a.createUserApp)
	authed.GET("/accessible-llms", a.getUserAccessibleLLMs)
	authed.GET("/apps/:id", a.getUserAppDetails)
	authed.DELETE("/apps/:id", a.deleteUserApp)
	authed.GET("/apps/:id/plugin-resources", a.getAppPluginResources)

	// CHAT FEATURES
	authed.GET("/data-catalogues/:id/datasources", a.getDataCatalogueDatasources)
	authed.GET("/agents", a.HandleListAgents)                // List accessible agents for current user
	authed.GET("/agents/:id", a.HandleGetAgent)              // Get specific agent
	authed.GET("/agents/:id/stream", a.HandleAgentSSE)       // Establish SSE connection for agent
	authed.POST("/agents/:id/message", a.HandleAgentMessage) // Send message to agent session
	// Use secure version for portal users that hides sensitive fields like auth_key and oas_spec
	authed.GET("/tool-catalogues/:id/tools", a.getToolCatalogueToolsSecure)
	// Route for tool documentation page
	authed.GET("/tools/:id/docs", a.GetToolDocumentation)
	// Route to get user apps that have access to a tool
	authed.GET("/tools/:id/user-apps", a.getToolUserApps)
	authed.GET("/users/:user_id/chat-history-records", a.getUserChatHistoryRecords)
	authed.GET("/accessible-datasources", a.getUserAccessibleDataSources)
	authed.GET("/accessible-tools", a.getUserAccessibleTools)
	authed.GET("/accessible-plugin-resources", a.getUserAccessiblePluginResources)
	authed.GET("/history", a.listChatHistoryRecordsForMe)
	authed.GET("/chat-sessions/:id/defaults", a.getChatDefaults)
	authed.GET("/sessions/:session_id/messages", a.getLastCMessagesForSession)
	authed.PUT("/chat-history-records/:session_id/name", a.updateChatHistoryRecordName)

	// Portal analytics endpoints with proper user validation
	authed.GET("/apps/:id/analytics/usage", a.getUserAppUsage)
	authed.GET("/apps/:id/analytics/interactions", a.getUserAppInteractions)

	// Notification routes
	notificationHandlers := NewNotificationHandlers(a.service.NotificationService)
	authed.GET("/api/v1/notifications", notificationHandlers.ListNotifications)
	authed.GET("/api/v1/notifications/unread/count", notificationHandlers.UnreadCount)
	authed.PUT("/api/v1/notifications/:id/read", notificationHandlers.MarkAsRead)
	authed.PUT("/api/v1/notifications/read-all", notificationHandlers.MarkAllAsRead)

	// Portal plugin routes (any authenticated user, no admin required)
	authed.GET("/plugins/portal-ui-registry", a.getPortalUIRegistry)
	authed.GET("/plugins/portal-sidebar-menu", a.getPortalSidebarMenuItems)
	authed.POST("/plugins/:id/portal-rpc/:method", a.callPortalPluginRPC)
	authed.GET("/plugins/assets/:id/*filepath", a.servePluginAsset)

	// UGC Submission routes (portal users)
	authed.POST("/submissions", a.createSubmission)
	authed.GET("/submissions", a.listMySubmissions)
	authed.GET("/submissions/:id", a.getMySubmission)
	authed.PATCH("/submissions/:id", a.updateMySubmission)
	authed.DELETE("/submissions/:id", a.deleteMySubmission)
	authed.POST("/submissions/:id/submit", a.submitSubmission)
	authed.GET("/submissions/attestation-templates", a.getAttestationTemplatesForSubmission)
	authed.POST("/submissions/update", a.createUpdateSubmission)
	authed.POST("/submissions/validate-spec", a.validateOASSpec)
	authed.GET("/submissions/:id/activities", a.getMySubmissionActivities)

	v1 := public.Group("/api/v1")
	v1.Use(a.auth.AuthMiddleware())
	v1.Use(a.auth.AdminOnly())

	// User routes
	v1.POST("/logout", a.handleLogout)
	v1.POST("/users", a.createUser)
	v1.GET("/users/:id", a.getUser)
	v1.PATCH("/users/:id", a.updateUser)
	v1.DELETE("/users/:id", a.deleteUser)
	v1.GET("/users", a.listUsers)
	v1.GET("/users/:id/catalogues", a.getUserAccessibleCatalogues)
	v1.POST("/users/:id/roll-api-key", a.rollUserAPIKey)
	v1.POST("/users/:id/skip-quick-start", a.skipUserQuickStart)

	// Group routes
	v1.POST("/groups", a.createGroup)
	v1.GET("/groups/:id", a.getGroup)
	v1.PATCH("/groups/:id", a.updateGroup)
	v1.DELETE("/groups/:id", a.deleteGroup)
	v1.GET("/groups", a.listGroups)
	v1.POST("/groups/:id/users", a.addUserToGroup)
	v1.DELETE("/groups/:id/users/:userId", a.removeUserFromGroup)
	v1.GET("/groups/:id/users", a.listGroupUsers)
	v1.POST("/groups/:id/catalogues", a.addCatalogueToGroup)
	v1.DELETE("/groups/:id/catalogues/:catalogueId", a.removeCatalogueFromGroup)
	v1.GET("/groups/:id/catalogues", a.listGroupCatalogues)
	v1.GET("/users/:id/groups", a.getUserGroups)
	v1.POST("/groups/:id/data-catalogues", a.addDataCatalogueToGroup)
	v1.DELETE("/groups/:id/data-catalogues/:dataCatalogueId", a.removeDataCatalogueFromGroup)
	v1.GET("/groups/:id/data-catalogues", a.listGroupDataCatalogues)
	v1.POST("/groups/:id/tool-catalogues", a.addToolCatalogueToGroup)
	v1.DELETE("/groups/:id/tool-catalogues/:toolCatalogueId", a.removeToolCatalogueFromGroup)
	v1.GET("/groups/:id/tool-catalogues", a.listGroupToolCatalogues)
	v1.PUT("/groups/:id/catalogues", a.updateGroupCatalogues)
	v1.PUT("/groups/:id/users", a.updateGroupUsers)

	// LLM routes
	v1.POST("/llms", a.createLLM)
	v1.GET("/llms/:id", a.getLLM)
	v1.PATCH("/llms/:id", a.updateLLM)
	v1.DELETE("/llms/:id", a.deleteLLM)
	v1.GET("/llms", a.listLLMs)
	v1.GET("/llms/search", a.searchLLMs)
	v1.GET("/llms/max-privacy-score", a.getLLMsByMaxPrivacyScore)
	v1.GET("/llms/min-privacy-score", a.getLLMsByMinPrivacyScore)
	v1.GET("/llms/privacy-score-range", a.getLLMsByPrivacyScoreRange)

	// Catalogue routes
	v1.POST("/catalogues", a.createCatalogue)
	v1.GET("/catalogues/:id", a.getCatalogue)
	v1.PATCH("/catalogues/:id", a.updateCatalogue)
	v1.DELETE("/catalogues/:id", a.deleteCatalogue)
	v1.GET("/catalogues", a.listCatalogues)
	v1.GET("/catalogues/search", a.searchCatalogues)
	v1.GET("/catalogues/search-by-stub", a.searchCataloguesByNameStub)
	v1.POST("/catalogues/:id/llms", a.addLLMToCatalogue)
	v1.DELETE("/catalogues/:id/llms/:llmId", a.removeLLMFromCatalogue)
	v1.GET("/catalogues/:id/llms", a.listCatalogueLLMs)

	// Tag routes
	v1.POST("/tags", a.createTag)
	v1.GET("/tags/:id", a.getTag)
	v1.PATCH("/tags/:id", a.updateTag)
	v1.DELETE("/tags/:id", a.deleteTag)
	v1.GET("/tags", a.listTags)
	v1.GET("/tags/search", a.searchTags)

	// Datasource routes
	v1.POST("/datasources", a.createDatasource)
	v1.GET("/datasources/:id", a.getDatasource)
	v1.PATCH("/datasources/:id", a.updateDatasource)
	v1.DELETE("/datasources/:id", a.deleteDatasource)
	v1.GET("/datasources", a.listDatasources)
	v1.GET("/datasources/search", a.searchDatasources)
	v1.GET("/datasources/by-tag", a.getDatasourcesByTag)
	v1.POST("/datasources/:id/filestores/:filestore_id", a.addFileStoreToDatasource)
	v1.DELETE("/datasources/:id/filestores/:filestore_id", a.removeFileStoreFromDatasource)
	v1.POST("/datasources/:id/process-embeddings", a.ProcessFileEmbeddingHandler)
	v1.POST("/datasources/:id/clone", a.cloneDatasource)

	// Data Catalogue routes
	v1.POST("/data-catalogues", a.createDataCatalogue)
	v1.GET("/data-catalogues/:id", a.getDataCatalogue)
	v1.PATCH("/data-catalogues/:id", a.updateDataCatalogue)
	v1.DELETE("/data-catalogues/:id", a.deleteDataCatalogue)
	v1.GET("/data-catalogues", a.listDataCatalogues)
	v1.GET("/data-catalogues/search", a.searchDataCatalogues)
	v1.POST("/data-catalogues/:id/tags", a.addTagToDataCatalogue)
	v1.DELETE("/data-catalogues/:id/tags/:tagId", a.removeTagFromDataCatalogue)
	v1.POST("/data-catalogues/:id/datasources", a.addDatasourceToDataCatalogue)
	v1.DELETE("/data-catalogues/:id/datasources/:datasourceId", a.removeDatasourceFromDataCatalogue)
	v1.GET("/data-catalogues/by-tag", a.getDataCataloguesByTag)
	v1.GET("/data-catalogues/by-datasource", a.getDataCataloguesByDatasource)

	// ToolCatalogue routes
	v1.POST("/tool-catalogues", a.createToolCatalogue)
	v1.GET("/tool-catalogues/:id", a.getToolCatalogue)
	v1.PATCH("/tool-catalogues/:id", a.updateToolCatalogue)
	v1.DELETE("/tool-catalogues/:id", a.deleteToolCatalogue)
	v1.GET("/tool-catalogues", a.listToolCatalogues)
	v1.GET("/tool-catalogues/search", a.searchToolCatalogues)
	v1.POST("/tool-catalogues/:id/tools", a.addToolToToolCatalogue)
	v1.DELETE("/tool-catalogues/:id/tools/:toolId", a.removeToolFromToolCatalogue)
	v1.GET("/tool-catalogues/:id/tools", a.getToolCatalogueTools)
	v1.POST("/tool-catalogues/:id/tags", a.addTagToToolCatalogue)
	v1.DELETE("/tool-catalogues/:id/tags/:tagId", a.removeTagFromToolCatalogue)
	v1.GET("/tool-catalogues/:id/tags", a.getToolCatalogueTags)

	// Credential routes
	v1.POST("/credentials", a.createCredential)
	v1.GET("/credentials/:id", a.getCredential)
	v1.GET("/credentials/key/:keyId", a.getCredentialByKeyID)
	v1.PATCH("/credentials/:id", a.updateCredential)
	v1.DELETE("/credentials/:id", a.deleteCredential)
	v1.POST("/credentials/:id/activate", a.activateCredential)
	v1.POST("/credentials/:id/deactivate", a.deactivateCredential)
	v1.GET("/credentials", a.listCredentials)
	v1.GET("/credentials/active", a.listActiveCredentials)

	// App routes
	v1.POST("/apps", a.createApp)
	v1.GET("/apps/:id", a.getApp)
	v1.PATCH("/apps/:id", a.updateApp)
	v1.DELETE("/apps/:id", a.deleteApp)
	v1.GET("/users/:id/apps", a.getAppsByUserID) // Note: Param is "id" here, not "userId" as in some other handlers
	v1.GET("/apps/by-name", a.getAppByName)
	v1.POST("/apps/:id/activate-credential", a.activateAppCredential)
	v1.POST("/apps/:id/deactivate-credential", a.deactivateAppCredential)
	v1.POST("/apps/:id/reset-budget", a.resetAppBudget)
	v1.GET("/apps", a.listApps)
	v1.GET("/apps/search", a.searchApps)
	v1.GET("/apps/count", a.countApps)
	v1.GET("/users/:id/apps/count", a.countAppsByUserID) // Note: Param is "id" here

	// App-Tool routes
	v1.POST("/apps/:id/tools/:tool_id", a.addToolToApp)
	v1.DELETE("/apps/:id/tools/:tool_id", a.removeToolFromApp)
	v1.GET("/apps/:id/tools", a.getAppTools)

	// Plugin Resource Type routes
	v1.GET("/plugin-resource-types", a.listPluginResourceTypes)
	v1.GET("/plugin-resource-types/:plugin_id/:slug/instances", a.listPluginResourceInstances)
	v1.GET("/apps/:id/plugin-resources", a.getAppPluginResources)
	v1.GET("/groups/:id/plugin-resources", a.getGroupPluginResources)
	v1.PUT("/groups/:id/plugin-resources", a.setGroupPluginResources)

	// LLMSettings routes
	v1.POST("/llm-settings", a.createLLMSettings)
	v1.GET("/llm-settings/:id", a.getLLMSettings)
	v1.PATCH("/llm-settings/:id", a.updateLLMSettings)
	v1.DELETE("/llm-settings/:id", a.deleteLLMSettings)
	v1.GET("/llm-settings", a.listLLMSettings)
	v1.GET("/llm-settings/search", a.searchLLMSettings)

	// Chat routes
	v1.POST("/chats", a.createChat)
	v1.GET("/chats/:id", a.getChat)
	v1.PATCH("/chats/:id", a.updateChat)
	v1.DELETE("/chats/:id", a.deleteChat)
	v1.GET("/chats", a.listChats)
	v1.GET("/chats/by-group", a.getChatsByGroupID)
	v1.POST("/chats/:id/extra-context/:filestore_id", a.addExtraContextToChat)
	v1.DELETE("/chats/:id/extra-context/:filestore_id", a.removeExtraContextFromChat)
	v1.GET("/chats/:id/extra-context", a.getChatExtraContext)
	v1.PUT("/chats/:id/extra-context", a.setChatExtraContext)

	// Prompt Template route
	v1.PATCH("/chats/:id/prompt-templates", a.updateChatPromptTemplates)

	// Agent routes
	v1.GET("/agents/:id/stream", a.HandleAgentSSE)             // Establish SSE connection for agent
	v1.POST("/agents/:id/message", a.HandleAgentMessage)       // Send message to agent session
	v1.GET("/agents", a.HandleListAgents)                      // List agent configs
	v1.GET("/agents/:id", a.HandleGetAgent)                    // Get agent config
	v1.POST("/agents", a.HandleCreateAgent)                    // Create agent config (admin only)
	v1.PUT("/agents/:id", a.HandleUpdateAgent)                 // Update agent config (admin only)
	v1.DELETE("/agents/:id", a.HandleDeleteAgent)              // Delete agent config (admin only)
	v1.POST("/agents/:id/activate", a.HandleActivateAgent)     // Activate agent (admin only)
	v1.POST("/agents/:id/deactivate", a.HandleDeactivateAgent) // Deactivate agent (admin only)

	// Tool routes
	v1.POST("/tools", a.createTool)
	v1.GET("/tools/:id", a.getTool)
	v1.PATCH("/tools/:id", a.updateTool)
	v1.DELETE("/tools/:id", a.deleteTool)
	v1.GET("/tools", a.getAllTools)
	v1.GET("/tools/by-type", a.getToolsByType)
	v1.GET("/tools/search", a.searchTools)
	v1.POST("/tools/:id/operations", a.addOperationToTool)
	v1.DELETE("/tools/:id/operations", a.removeOperationFromTool)
	v1.GET("/tools/:id/operations", a.getToolOperations)

	v1.GET("/tools/:id/spec-operations", a.listToolSpecOperations)
	v1.POST("/tools/:id/call-operation", a.callToolOperation)

	v1.POST("/tools/:id/dependencies/:dependency_id", a.addDependencyToTool)
	v1.DELETE("/tools/:id/dependencies/:dependency_id", a.removeDependencyFromTool)
	v1.GET("/tools/:id/dependencies", a.getToolDependencies)
	v1.PUT("/tools/:id/dependencies", a.setToolDependencies)

	v1.POST("/tools/:id/filestores/:filestore_id", a.addFileStoreToTool)
	v1.DELETE("/tools/:id/filestores/:filestore_id", a.removeFileStoreFromTool)
	v1.GET("/tools/:id/filestores", a.getToolFileStores)
	v1.PUT("/tools/:id/filestores", a.setToolFileStores)

	v1.POST("/tools/:id/filters/:filter_id", a.addFilterToTool)
	v1.DELETE("/tools/:id/filters/:filter_id", a.removeFilterFromTool)
	v1.GET("/tools/:id/filters", a.getToolFilters)
	v1.PUT("/tools/:id/filters", a.setToolFilters)

	// Provider routes
	providerAPI := NewProviderAPI(a)
	providerAPI.RegisterRoutes(v1)

	// Model Price routes
	v1.POST("/model-prices", a.createModelPrice)
	v1.GET("/model-prices/:id", a.getModelPrice)
	v1.PATCH("/model-prices/:id", a.updateModelPrice)
	v1.PATCH("/model-prices/:id/recalculate", a.updateModelPriceAndRecalculate)
	v1.DELETE("/model-prices/:id", a.deleteModelPrice)
	v1.GET("/model-prices", a.getAllModelPrices)
	v1.GET("/model-prices/by-vendor", a.getModelPricesByVendor)
	v1.GET("/model-prices/by-name", a.getOrCreateModelPriceByName)

	// Vendor routes
	v1.GET("/vendors/llm-drivers", a.getAvailableLLMDrivers)
	v1.GET("/vendors/embedders", a.getAvailableEmbedders)
	v1.GET("/vendors/vector-stores", a.getAvailableVectorStores)

	// Filter routes
	v1.POST("/filters", a.createFilter)
	v1.GET("/filters/:id", a.getFilter)
	v1.PATCH("/filters/:id", a.updateFilter)
	v1.DELETE("/filters/:id", a.deleteFilter)
	v1.GET("/filters", a.listFilters)
	v1.POST("/filters/test", a.testFilter)

	// Plugin routes
	v1.POST("/plugins", a.createPlugin)
	v1.GET("/plugins/:id", a.getPlugin)
	v1.PATCH("/plugins/:id", a.updatePlugin)
	v1.DELETE("/plugins/:id", a.deletePlugin)
	v1.DELETE("/plugins/:id/data", a.clearPluginData)
	v1.GET("/plugins", a.listPlugins)
	v1.POST("/plugins/:id/test", a.testPlugin)

	// OCI Plugin routes
	v1.POST("/plugins/oci", a.createOCIPlugin)
	v1.GET("/plugins/oci/cached", a.listCachedOCIPlugins)
	v1.POST("/plugins/:id/refresh", a.refreshOCIPlugin)
	v1.GET("/plugins/type/:type", a.getPluginsByType)
	v1.GET("/plugins/ai-studio/manifests", a.getAIStudioPluginsWithManifests)

	// Plugin UI Management routes
	v1.GET("/plugins/ui-registry", a.getUIRegistry)
	v1.GET("/plugins/sidebar-menu", a.getSidebarMenuItems)
	v1.POST("/plugins/:id/ui/load", a.loadPluginUI)
	v1.POST("/plugins/:id/ui/unload", a.unloadPluginUI)
	v1.POST("/plugins/:id/manifest/parse", a.parsePluginManifest)

	// Plugin RPC routes
	v1.POST("/plugins/:id/rpc/:method", a.callPluginRPC)
	v1.POST("/plugins/:id/reload", a.reloadPlugin)

	// Plugin runtime status routes (for debugging)
	v1.GET("/plugins/:id/status", a.getPluginStatus)
	v1.GET("/plugins/loaded", a.getLoadedPlugins)

	// Plugin configuration schema routes
	v1.GET("/plugins/:id/config-schema", a.getPluginConfigSchema)
	v1.POST("/plugins/:id/config-schema/refresh", a.refreshPluginConfigSchema)

	// Plugin workflow routes (for step-by-step creation and approval)
	v1.POST("/plugins/:id/validate-and-load", a.validateAndLoadPlugin)
	v1.POST("/plugins/:id/approve-scopes", a.approvePluginScopes)
	v1.GET("/plugins/:id/workflow-status", a.getPluginWorkflowStatus)

	// Plugin cleanup routes
	v1.POST("/plugins/cleanup-orphaned-registry", a.cleanupOrphanedUIRegistry)

	// Plugin schedule routes
	v1.POST("/plugins/:id/schedules", a.CreatePluginSchedule)
	v1.GET("/plugins/:id/schedules", a.GetPluginSchedules)
	v1.GET("/plugins/:id/schedules/:schedule_id", a.GetPluginScheduleDetail)
	v1.GET("/plugins/:id/schedules/:schedule_id/executions", a.GetPluginScheduleExecutions)
	v1.PUT("/plugins/:id/schedules/:schedule_id", a.UpdatePluginSchedule)
	v1.DELETE("/plugins/:id/schedules/:schedule_id", a.DeletePluginSchedule)

	// Plugin asset serving (outside of v1 group for simpler URLs)
	v1.GET("/plugins/assets/:id/*filepath", a.servePluginAsset)

	// LLM-Plugin association routes (extend existing LLM routes)
	v1.GET("/llms/:id/plugins", a.getLLMPlugins)
	v1.PUT("/llms/:id/plugins", a.updateLLMPlugins)

	// LLM-Plugin configuration routes
	v1.GET("/llms/:id/plugins/:pluginId/config", a.getLLMPluginConfig)
	v1.PUT("/llms/:id/plugins/:pluginId/config", a.updateLLMPluginConfig)

	// Model Router routes (Enterprise only)
	v1.POST("/model-routers", a.createModelRouter)
	v1.GET("/model-routers/:id", a.getModelRouter)
	v1.PATCH("/model-routers/:id", a.updateModelRouter)
	v1.DELETE("/model-routers/:id", a.deleteModelRouter)
	v1.GET("/model-routers", a.listModelRouters)
	v1.PATCH("/model-routers/:id/toggle", a.toggleModelRouterActive)

	// Marketplace routes (only register if marketplace service is available)
	if a.service.MarketplaceService != nil {
		marketplaceHandlers := NewMarketplaceHandlers(a.service.MarketplaceService)
		v1.GET("/marketplace/plugins", marketplaceHandlers.ListPlugins)
		v1.GET("/marketplace/plugins/:id", marketplaceHandlers.GetPlugin)
		v1.GET("/marketplace/plugins/:id/versions", marketplaceHandlers.GetPluginVersions)
		v1.GET("/marketplace/plugins/:id/install-metadata", marketplaceHandlers.GetInstallMetadata)
		v1.GET("/marketplace/updates", marketplaceHandlers.GetAvailableUpdates)
		v1.POST("/marketplace/sync", marketplaceHandlers.SyncMarketplace)
		v1.GET("/marketplace/sync-status", marketplaceHandlers.GetSyncStatus)
		v1.GET("/marketplace/categories", marketplaceHandlers.GetCategories)
		v1.GET("/marketplace/publishers", marketplaceHandlers.GetPublishers)
		v1.GET("/marketplace/stats", marketplaceHandlers.GetStats)
	}

	// Marketplace Admin routes (ENT: full management, CE: 403 responses)
	// Admin-only endpoints for managing multiple marketplace sources
	marketplaceAdmin := v1.Group("/admin/marketplaces")
	marketplaceAdmin.Use(a.auth.AdminOnly())
	{
		adminHandlers := NewMarketplaceAdminHandlers(a.marketplaceManagementService, a.service.MarketplaceService)
		marketplaceAdmin.POST("", adminHandlers.AddMarketplace)
		marketplaceAdmin.GET("", adminHandlers.ListMarketplaces)
		marketplaceAdmin.GET("/:id", adminHandlers.GetMarketplace)
		marketplaceAdmin.PUT("/:id", adminHandlers.UpdateMarketplace)
		marketplaceAdmin.DELETE("/:id", adminHandlers.RemoveMarketplace)
		marketplaceAdmin.POST("/validate", adminHandlers.ValidateMarketplaceURL)
		marketplaceAdmin.POST("/:id/sync", adminHandlers.SyncMarketplace)
	}

	// UGC Submission routes (admin review)
	v1.GET("/submissions", a.adminListSubmissions)
	v1.GET("/submissions/:id", a.adminGetSubmission)
	v1.POST("/submissions/:id/review", a.adminStartReview)
	v1.POST("/submissions/:id/approve", a.adminApproveSubmission)
	v1.POST("/submissions/:id/reject", a.adminRejectSubmission)
	v1.POST("/submissions/:id/request-changes", a.adminRequestChanges)
	v1.POST("/submissions/:id/test", a.adminTestSubmission)
	v1.GET("/submissions/:id/versions", a.adminListVersions)
	v1.POST("/submissions/:id/rollback/:version_id", a.adminRollbackVersion)
	v1.GET("/submissions/orphaned", a.adminGetOrphanedResources)
	v1.GET("/submissions/:id/activities", a.adminGetSubmissionActivities)
	v1.POST("/submissions/test-datasource", a.testDatasourceConnectivity)

	// Attestation Template routes (admin)
	v1.GET("/attestation-templates", a.adminListAttestationTemplates)
	v1.GET("/attestation-templates/:id", a.adminGetAttestationTemplate)
	v1.POST("/attestation-templates", a.adminCreateAttestationTemplate)
	v1.PATCH("/attestation-templates/:id", a.adminUpdateAttestationTemplate)
	v1.DELETE("/attestation-templates/:id", a.adminDeleteAttestationTemplate)

	// Chat History Record routes
	v1.POST("/chat-history-records", a.createChatHistoryRecord)
	v1.GET("/chat-history-records/messages/:session_id", a.getCMessagesForSession)
	v1.GET("/chat-history-records/:id", a.getChatHistoryRecord)
	v1.GET("/chat-history-records", a.listChatHistoryRecords)
	v1.DELETE("/chat-history-records/:id", a.deleteChatHistoryRecord)

	// Analytics routes
	v1.GET("/analytics/chat-records-per-day", a.getChatRecordsPerDay)
	v1.GET("/analytics/tool-calls-per-day", a.getToolCallsPerDay)
	v1.GET("/analytics/chat-records-per-user", a.getChatRecordsPerUser)
	v1.GET("/analytics/cost-analysis", a.getCostAnalysis)
	v1.GET("/analytics/most-used-llm-models", a.getMostUsedLLMModels)
	v1.GET("/analytics/tool-usage-statistics", a.getToolUsageStatistics)
	v1.GET("/analytics/tool-operations-usage-over-time", a.getToolOperationsUsageOverTime)
	v1.GET("/analytics/unique-users-per-day", a.getUniqueUsersPerDay)
	v1.GET("/analytics/token-usage-per-user", a.getTokenUsagePerUser)
	v1.GET("/analytics/token-usage-per-app", a.getTokenUsagePerApp)
	v1.GET("/analytics/token-usage-for-app", a.getTokenUsageForApp)
	v1.GET("/analytics/usage", a.getUsage)
	v1.GET("/analytics/token-usage-and-cost-for-app", a.getTokenUsageAndCostForApp)
	v1.GET("/analytics/chat-interactions-for-chat", a.getChatInteractionsForChat)
	v1.GET("/analytics/model-usage", a.getModelUsage)
	v1.GET("/analytics/vendor-usage", a.getVendorUsage)
	v1.GET("/analytics/total-cost-per-vendor-and-model", a.getTotalCostPerVendorAndModel)
	v1.GET("/analytics/budget-usage", a.getBudgetUsage)
	v1.GET("/analytics/budget-usage-for-app", a.getBudgetUsageForApp)
	v1.GET("/analytics/app-interactions-over-time", a.getAppInteractionsOverTime)

	v1.GET("/analytics/proxy-logs-for-app", a.getProxyLogsForApp)
	v1.GET("/analytics/proxy-logs-for-llm", a.getProxyLogsForLLM)

	// Compliance routes (Enterprise feature)
	a.InitComplianceService()
	v1.GET("/compliance/available", a.isComplianceAvailable)
	v1.GET("/compliance/summary", a.getComplianceSummary)
	v1.GET("/compliance/high-risk-apps", a.getHighRiskApps)
	v1.GET("/compliance/access-issues", a.getAccessIssues)
	v1.GET("/compliance/policy-violations", a.getPolicyViolations)
	v1.GET("/compliance/violations", a.getViolationRecords)
	v1.GET("/compliance/budget-alerts", a.getBudgetAlerts)
	v1.GET("/compliance/errors", a.getComplianceErrors)
	v1.GET("/compliance/app/:id/risk-profile", a.getAppRiskProfile)
	v1.GET("/compliance/export", a.exportComplianceData)

	// Export routes (Enterprise feature)
	v1.POST("/exports", a.startExport)
	v1.GET("/exports/:id", a.getExport)
	v1.GET("/exports/:id/download", a.downloadExport)

	// FileStore routes
	v1.POST("/filestore", a.createFileStore)
	v1.GET("/filestore/:id", a.getFileStore)
	v1.PATCH("/filestore/:id", a.updateFileStore)
	v1.DELETE("/filestore/:id", a.deleteFileStore)
	v1.GET("/filestore", a.getAllFileStores)
	v1.GET("/filestore/search", a.searchFileStores)

	v1.POST("/secrets", a.createSecret)
	v1.GET("/secrets/:id", a.getSecret)
	v1.PATCH("/secrets/:id", a.updateSecret)
	v1.DELETE("/secrets/:id", a.deleteSecret)
	v1.GET("/secrets", a.listSecrets)

	// Branding routes
	// Public endpoints (for frontend config loading and asset serving)
	public.GET("/api/v1/branding/settings", a.getBrandingSettings)
	public.GET("/api/v1/branding/logo", a.serveLogo)
	public.GET("/api/v1/branding/favicon", a.serveFavicon)
	// Admin-only endpoints (for customization)
	v1.PUT("/branding/settings", a.updateBrandingSettings)
	v1.POST("/branding/logo", a.uploadLogo)
	v1.POST("/branding/favicon", a.uploadFavicon)
	v1.POST("/branding/reset", a.resetBranding)

	// Edge Management routes (Hub-and-Spoke)
	v1.GET("/edges", a.listEdges)
	v1.GET("/edges/:edge_id", a.getEdge)
	v1.POST("/edges/:edge_id/reload", a.triggerEdgeReload)
	v1.POST("/edges/reload-all", a.reloadAllEdges) // CE: works (single namespace), ENT: works (all namespaces)
	v1.GET("/edges/reload-operations", a.listReloadOperations)
	v1.GET("/reload-operations/:operation_id/status", a.getReloadOperationStatus)
	v1.DELETE("/edges/:edge_id", a.deleteEdge)

	// Namespace Management routes (Hub-and-Spoke)
	v1.GET("/namespaces", a.listNamespaces)
	v1.POST("/namespaces/:namespace/reload", a.triggerNamespaceReload)
	v1.GET("/namespaces/:namespace/edges", a.getNamespaceEdges)

	// Sync status routes (admin only - for edge gateway sync monitoring)
	syncStatusHandlers := NewSyncStatusHandlers(a.service.SyncStatusService)
	v1.GET("/sync/status", syncStatusHandlers.GetSyncStatus)
	v1.GET("/sync/status/:namespace", syncStatusHandlers.GetNamespaceSyncStatus)
	v1.GET("/sync/audit", syncStatusHandlers.GetSyncAuditLog)

	// SSO routes (ENT: full functionality, CE: returns 402 Payment Required)
	public.GET("/auth/:id/:provider", a.handleTIBAuth)
	public.POST("/auth/:id/:provider", a.handleTIBAuth)
	public.GET("/auth/:id/:provider/callback", a.handleTIBAuthCallback)
	public.POST("/auth/:id/:provider/callback", a.handleTIBAuthCallback)
	public.GET("/auth/:id/saml/metadata", a.handleSAMLMetadata)
	public.POST("/auth/:id/saml/metadata", a.handleSAMLMetadata)
	public.GET("/sso", a.handleSSO)
	public.GET("/login-sso-profile", a.getLoginPageProfile)

	apiGroup := public.Group("/api")
	apiGroup.Use(a.SSOAuthMiddleware())
	apiGroup.POST("/sso", a.handleNonceRequest)

	profiles := v1.Group("/sso-profiles")
	profiles.Use(a.auth.SSOOnly())
	profiles.POST("", a.createProfile)
	profiles.GET("", a.listProfiles)
	profiles.GET("/:profile_id", a.getProfile)
	profiles.PUT("/:profile_id", a.updateProfile)
	profiles.DELETE("/:profile_id", a.deleteProfile)
	profiles.POST("/:profile_id/use-in-login-page", a.setProfileUseInLoginPage)

	// Always enable chat features
	if a.setupChatRoutesFunc != nil {
		a.setupChatRoutesFunc(authed)
	}
}

func (a *API) devCorsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token", "Last-Event-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Total-Count", "X-Total-Pages", "X-CSRF-Token", "Last-Event-ID", "Location"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func (a *API) corsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token", "Last-Event-ID"},
		ExposeHeaders: []string{
			"Content-Length", "X-Total-Count", "X-Total-Pages", "X-CSRF-Token", "Last-Event-ID",
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func (a *API) handleGetConfig(c *gin.Context) {
	scheme := "http"

	host := c.Request.Host
	siteURLVar := os.Getenv("SITE_URL")
	if siteURLVar != "" {
		asURL, err := url.Parse(siteURLVar)
		if err == nil {
			host = asURL.Host
			scheme = asURL.Scheme
		}
	}

	apiBaseURL := fmt.Sprintf("%s://%s", scheme, host)

	suMode := "both"
	if config.Get("").DefaultSignupMode != "" {
		suMode = config.Get("").DefaultSignupMode
	}

	// Get branding settings
	var brandingConfig *BrandingConfig
	if a.service != nil {
		brandingSettings, err := a.service.GetBrandingSettings()
		if err == nil {
			brandingConfig = &BrandingConfig{
				AppTitle:         brandingSettings.AppTitle,
				PrimaryColor:     brandingSettings.PrimaryColor,
				SecondaryColor:   brandingSettings.SecondaryColor,
				BackgroundColor:  brandingSettings.BackgroundColor,
				CustomCSS:        brandingSettings.CustomCSS,
				HasCustomLogo:    brandingSettings.HasCustomLogo(),
				HasCustomFavicon: brandingSettings.HasCustomFavicon(),
			}
		}
	}

	// Get display URLs with fallback to ProxyURL
	proxyURL := config.Get("").ProxyURL
	toolDisplayURL := config.Get("").ToolDisplayURL
	if toolDisplayURL == "" {
		toolDisplayURL = proxyURL
	}
	dataSourceDisplayURL := config.Get("").DataSourceDisplayURL
	if dataSourceDisplayURL == "" {
		dataSourceDisplayURL = proxyURL
	}

	cfg := FrontendConfig{
		APIBaseURL:           apiBaseURL,
		ProxyURL:             proxyURL,
		ToolDisplayURL:       toolDisplayURL,
		DataSourceDisplayURL: dataSourceDisplayURL,
		DefaultSignUpMode:    suMode,
		TIBEnabled:           sso.IsEnterpriseAvailable(),
		IsEnterprise:         config.IsEnterprise(), // Detect enterprise edition via build tags
		DocsLinks:            config.Get("").DocsLinks,
		Branding:             brandingConfig,
		DocsEnabled:          !config.Get("").DocsDisabled,
		DocsURL:              config.Get("").DocsURL,
	}

	c.JSON(http.StatusOK, cfg)
}
