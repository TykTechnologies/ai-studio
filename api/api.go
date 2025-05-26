package api

import (
	"bytes"
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
	"github.com/TykTechnologies/midsommar/v2/licensing"
	"github.com/TykTechnologies/midsommar/v2/providers"
	"github.com/TykTechnologies/midsommar/v2/providers/tyk"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
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
	service             *services.Service
	router              *gin.Engine
	config              *auth.Config
	disableCORS         bool
	auth                *auth.AuthService
	proxy               *proxy.Proxy
	staticFiles         embed.FS
	providers           *providers.Registry
	setupChatRoutesFunc func(*gin.RouterGroup)
	ssoService          *services.SSOService
	licenser            *licensing.Licenser
}

func NewAPI(service *services.Service, disableCORS bool, authService *auth.AuthService, config *auth.Config, proxy *proxy.Proxy, staticFiles embed.FS, licenser *licensing.Licenser) *API {
	gin.SetMode(gin.ReleaseMode)

	// Initialize provider registry
	providerRegistry := providers.NewRegistry()

	// Register the Tyk Dashboard provider by default
	tykProvider := tyk.NewTykDashboardProvider(providers.ProviderConfig{})
	if err := providerRegistry.RegisterProvider("tyk", tykProvider); err != nil {
		log.Printf("Failed to register Tyk provider: %v", err)
	}

	router := gin.Default()

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
		service:     service,
		router:      router,
		disableCORS: disableCORS,
		auth:        authService,
		config:      config,
		proxy:       proxy,
		staticFiles: staticFiles,
		providers:   providerRegistry,
		licenser:    licenser,
	}

	if config.TIBEnabled {
		logLevel := "info"

		if config.TestMode {
			logLevel = "debug"
		}

		ssoConfig := &services.Config{
			APISecret: config.TIBAPISecret,
			LogLevel:  logLevel,
		}
		api.ssoService = services.NewSSOService(ssoConfig, router, config.DB, service.NotificationService)

		api.ssoService.InitInternalTIB()
	}

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
	if certFile != "" && keyFile != "" {
		return a.router.RunTLS(addr, certFile, keyFile)
	}

	return a.router.Run(addr)
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

	a.router.Use(a.licenser.TelemetryMiddleware())

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

	public := a.router.Group("/")

	// Public routes
	public.POST("/auth/login", a.handleLogin)
	public.POST("/auth/register", a.handleRegister)
	public.POST("/auth/forgot-password", a.handleForgotPassword)
	public.POST("/auth/reset-password", a.handleResetPassword)
	public.GET("/auth/validate-reset-token", a.handleValidateResetToken)
	public.GET("/auth/verify-email", a.handleVerifyEmail)
	public.POST("/auth/resend-verification", a.handleResendVerification)
	public.GET("/auth/config", a.handleGetConfig)
	public.GET("/auth/features", a.handleFeatureSet)

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

	// CHAT FEATURES
	authed.GET("/data-catalogues/:id/datasources", a.getDataCatalogueDatasources)
	authed.GET("/tool-catalogues/:id/tools", a.getToolCatalogueTools)
	authed.GET("/users/:user_id/chat-history-records", a.getUserChatHistoryRecords)
	authed.GET("/accessible-datasources", a.getUserAccessibleDataSources)
	authed.GET("/accessible-tools", a.getUserAccessibleTools)
	authed.GET("/history", a.listChatHistoryRecordsForMe)
	authed.GET("/chat-sessions/:id/defaults", a.getChatDefaults)
	authed.GET("/sessions/:session_id/messages", a.getLastCMessagesForSession)
	authed.PUT("/chat-history-records/:session_id/name", a.updateChatHistoryRecordName)

	// Notification routes
	notificationHandlers := NewNotificationHandlers(a.service.NotificationService)
	authed.GET("/api/v1/notifications", notificationHandlers.ListNotifications)
	authed.GET("/api/v1/notifications/unread/count", notificationHandlers.UnreadCount)
	authed.PUT("/api/v1/notifications/:id/read", notificationHandlers.MarkAsRead)

	v1 := public.Group("/api/v1")
	v1.Use(a.auth.AuthMiddleware())
	v1.Use(a.auth.AdminOnly())

	// User routes
	v1.POST("/logout", a.handleLogout)
	v1.POST("/users", licensing.ActionHandler(a.createUser, "Create User"))
	v1.GET("/users/:id", licensing.ActionHandler(a.getUser, "Get User"))
	v1.PATCH("/users/:id", licensing.ActionHandler(a.updateUser, "Update User"))
	v1.DELETE("/users/:id", licensing.ActionHandler(a.deleteUser, "Delete User"))
	v1.GET("/users", a.listUsers)
	v1.GET("/users/:id/catalogues", a.getUserAccessibleCatalogues)
	v1.POST("/users/:id/roll-api-key", a.rollUserAPIKey)
	v1.POST("/users/:id/skip-quick-start", a.skipUserQuickStart)

	// Group routes
	v1.POST("/groups", licensing.ActionHandler(a.createGroup, "Create User Group"))
	v1.GET("/groups/:id", licensing.ActionHandler(a.getGroup, "Get User Group"))
	v1.PATCH("/groups/:id", licensing.ActionHandler(a.updateGroup, "Update User Group"))
	v1.DELETE("/groups/:id", licensing.ActionHandler(a.deleteGroup, "Delete User Group"))
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
	v1.PUT("/groups/:id/users", a.updateGroupUsers)
	v1.PUT("/groups/:id/catalogs", a.updateGroupCatalogs)

	// LLM routes
	v1.POST("/llms", licensing.ActionHandler(a.createLLM, "Create LLM"))
	v1.GET("/llms/:id", licensing.ActionHandler(a.getLLM, "Get LLM"))
	v1.PATCH("/llms/:id", licensing.ActionHandler(a.updateLLM, "Update LLM"))
	v1.DELETE("/llms/:id", licensing.ActionHandler(a.deleteLLM, "Delete LLM"))
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
	v1.POST("/apps", licensing.ActionHandler(a.createApp, "Create App"))
	v1.GET("/apps/:id", licensing.ActionHandler(a.getApp, "Get App"))
	v1.PATCH("/apps/:id", licensing.ActionHandler(a.updateApp, "Update App"))
	v1.DELETE("/apps/:id", licensing.ActionHandler(a.deleteApp, "Delete App"))
	v1.GET("/users/:id/apps", a.getAppsByUserID)
	v1.GET("/apps/by-name", a.getAppByName)
	v1.POST("/apps/:id/activate-credential", a.activateAppCredential)
	v1.POST("/apps/:id/deactivate-credential", a.deactivateAppCredential)
	v1.GET("/apps", a.listApps)
	v1.GET("/apps/search", a.searchApps)
	v1.GET("/apps/count", a.countApps)
	v1.GET("/users/:id/apps/count", a.countAppsByUserID)

	// LLMSettings routes
	v1.POST("/llm-settings", a.createLLMSettings)
	v1.GET("/llm-settings/:id", a.getLLMSettings)
	v1.PATCH("/llm-settings/:id", a.updateLLMSettings)
	v1.DELETE("/llm-settings/:id", a.deleteLLMSettings)
	v1.GET("/llm-settings", a.listLLMSettings)
	v1.GET("/llm-settings/search", a.searchLLMSettings)

	// Chat routes
	v1.POST("/chats", licensing.ActionHandler(a.createChat, "Create Chat"))
	v1.GET("/chats/:id", licensing.ActionHandler(a.getChat, "Get Chat"))
	v1.PATCH("/chats/:id", licensing.ActionHandler(a.updateChat, "Update Chat"))
	v1.DELETE("/chats/:id", licensing.ActionHandler(a.deleteChat, "Delete Chat"))
	v1.GET("/chats", a.listChats)
	v1.GET("/chats/by-group", a.getChatsByGroupID)
	v1.POST("/chats/:id/extra-context/:filestore_id", a.addExtraContextToChat)
	v1.DELETE("/chats/:id/extra-context/:filestore_id", a.removeExtraContextFromChat)
	v1.GET("/chats/:id/extra-context", a.getChatExtraContext)
	v1.PUT("/chats/:id/extra-context", a.setChatExtraContext)

	// Prompt Template route
	v1.PATCH("/chats/:id/prompt-templates", a.updateChatPromptTemplates)

	// Tool routes
	v1.POST("/tools", licensing.ActionHandler(a.createTool, "Create Tool"))
	v1.GET("/tools/:id", licensing.ActionHandler(a.getTool, "Get Tool"))
	v1.PATCH("/tools/:id", licensing.ActionHandler(a.updateTool, "Update Tool"))
	v1.DELETE("/tools/:id", licensing.ActionHandler(a.deleteTool, "Delete Tool"))
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

	v1.GET("/analytics/proxy-logs-for-app", a.getProxyLogsForApp)
	v1.GET("/analytics/proxy-logs-for-llm", a.getProxyLogsForLLM)

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

	// SSO routes
	if a.config.TIBEnabled {
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
	}

	chatEnabled, chaOK := a.licenser.Entitlement(licensing.FEATUREChat)
	if chaOK && chatEnabled.Bool() && a.setupChatRoutesFunc != nil {
		a.setupChatRoutesFunc(authed)
	}
}

func (a *API) devCorsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token", "Last-Event-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Total-Count", "X-Total-Pages", "X-CSRF-Token", "Last-Event-ID"},
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
	if config.Get().DefaultSignupMode != "" {
		suMode = config.Get().DefaultSignupMode
	}

	config := FrontendConfig{
		APIBaseURL:        apiBaseURL,
		ProxyURL:          config.Get().ProxyURL,
		DefaultSignUpMode: suMode,
		TIBEnabled:        a.config.TIBEnabled,
		DocsLinks:         config.Get().DocsLinks,
	}

	c.JSON(http.StatusOK, config)
}
