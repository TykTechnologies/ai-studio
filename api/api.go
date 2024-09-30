package api

import (
	"crypto/rand"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

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
	service     *services.Service
	router      *gin.Engine
	config      *auth.Config
	disableCORS bool
	auth        *auth.AuthService
}

func NewAPI(service *services.Service, disableCORS bool, authService *auth.AuthService, config *auth.Config) *API {
	gin.SetMode(gin.ReleaseMode)
	api := &API{
		service:     service,
		router:      gin.Default(),
		disableCORS: disableCORS,
		auth:        authService,
		config:      config,
	}

	// Generate a random 32-byte key for CSRF
	csrfKey := make([]byte, 32)
	_, err := rand.Read(csrfKey)
	if err != nil {
		log.Fatalf("Failed to generate CSRF key: %v", err)
	}

	// Add CSRF middleware
	csrfMiddleware := csrf.Protect(
		csrfKey,
		csrf.Secure(true), // Set to false for HTTP in development
		csrf.Path("/"),
	)

	api.router.Use(func(c *gin.Context) {
		csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Request = r
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)
	})

	api.setupRoutes()
	return api
}

func (a *API) Run(addr string, certFile string, keyFile string) error {
	if certFile != "" && keyFile != "" {
		return a.router.RunTLS(addr, certFile, keyFile)
	}

	return a.router.Run(addr)
}

// getPaginationParams extracts pagination parameters from the request
// If no parameters are provided, it returns default values for "all" pagination
func getPaginationParams(c *gin.Context) (int, int, bool) {
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "0"))
	pageNumber, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	allStr := c.DefaultQuery("all", "false")
	all := false

	if strings.ToLower(allStr) == "true" {
		all = true
	}

	if pageSize == 0 {
		// divide by zero!
		pageSize = 1
		all = true
	}

	return pageSize, pageNumber, all
}
func (a *API) setupRoutes() {
	if a.disableCORS {
		a.router.Use(a.devCorsMiddleware())
	} else {
		a.router.Use(a.corsMiddleware())
	}

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
	public.GET("/auth/verify-email", a.handleVerifyEmail)
	public.POST("/auth/resend-verification", a.handleResendVerification)

	// routes for portal users
	authed := public.Group("/common")
	authed.Use(a.auth.AuthMiddleware())
	authed.POST("/logout", a.handleLogout)
	authed.GET("/me", a.handleMe)
	authed.GET("/catalogues/:id/llms", a.getCatalogueLLMs)
	authed.GET("/data-catalogues/:id/datasources", a.getDataCatalogueDatasources)
	authed.GET("/tool-catalogues/:id/tools", a.getToolCatalogueTools)
	authed.GET("/users/:user_id/chat-history-records", a.getUserChatHistoryRecords)
	authed.GET("/apps", a.getUserApps)
	authed.POST("/apps", a.createUserApp)
	authed.GET("/accessible-datasources", a.getUserAccessibleDataSources)
	authed.GET("/accessible-tools", a.getUserAccessibleTools)
	authed.GET("/accessible-llms", a.getUserAccessibleLLMs)
	authed.GET("/apps/:id", a.getUserAppDetails)
	authed.DELETE("/apps/:id", a.deleteUserApp)
	authed.GET("/history", a.listChatHistoryRecordsForMe)
	authed.POST("/chat-sessions/:session_id/datasources", a.addDatasourceToChatSession)
	authed.DELETE("/chat-sessions/:session_id/datasources/:datasource_id", a.removeDatasourceFromChatSession)
	authed.POST("/chat-sessions/:session_id/tools", a.addToolToChatSession)
	authed.DELETE("/chat-sessions/:session_id/tools/:tool_id", a.removeToolFromChatSession)
	authed.POST("/chat-sessions/:session_id/upload", a.UploadFileToSession)
	authed.GET("/sessions/:session_id/messages", a.getLastCMessagesForSession)
	authed.PUT("/chat-history-records/:session_id/name", a.updateChatHistoryRecordName)

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
	v1.GET("/catalogues/search-by-stub", a.searchCataloguesByNameStub) // Add this line
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
	v1.POST("/chats", a.createChat)
	v1.GET("/chats/:id", a.getChat)
	v1.PATCH("/chats/:id", a.updateChat)
	v1.DELETE("/chats/:id", a.deleteChat)
	v1.GET("/chats", a.listChats)
	v1.GET("/chats/by-group", a.getChatsByGroupID)

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

	// Model Price routes
	v1.POST("/model-prices", a.createModelPrice)
	v1.GET("/model-prices/:id", a.getModelPrice)
	v1.PATCH("/model-prices/:id", a.updateModelPrice)
	v1.DELETE("/model-prices/:id", a.deleteModelPrice)
	v1.GET("/model-prices", a.getAllModelPrices)
	v1.GET("/model-prices/by-vendor", a.getModelPricesByVendor)

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
	v1.GET("/analytics/token-usage-and-cost-for-app", a.getTokenUsageAndCostForApp)
	v1.GET("/analytics/chat-interactions-for-chat", a.getChatInteractionsForChat)
	v1.GET("/analytics/model-usage", a.getModelUsage)
	v1.GET("/analytics/vendor-usage", a.getVendorUsage)
	v1.GET("/analytics/total-cost-per-vendor-and-model", a.getTotalCostPerVendorAndModel)

	a.SetupWebSocketRoute(authed)
}

func (a *API) devCorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Allow the specific origin
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)

		// Allow credentials
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		// Allow specific headers
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Total-Count, X-Total-Pages")

		// Expose headers
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Total-Count, X-Total-Pages")

		// Allow methods
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (a *API) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cors.New(cors.Config{
			AllowOrigins:     []string{"http://localhost:3000"}, // Update with your frontend URL
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token"},
			ExposeHeaders:    []string{"Content-Length", "X-Total-Count", "X-Total-Pages", "X-CSRF-Token"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		})(c)
	}
}

func (a *API) GetUserID() uint {
	return 0
}
