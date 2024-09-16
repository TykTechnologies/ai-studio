package api

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
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
	service *services.Service
	router  *gin.Engine
}

func NewAPI(service *services.Service) *API {
	api := &API{
		service: service,
		router:  gin.Default(),
	}
	api.setupRoutes()
	return api
}

func (a *API) Run(addr string) error {
	return a.router.Run(addr)
}

func (a *API) setupRoutes() {
	a.router.Use(a.authMiddleware())

	v1 := a.router.Group("/api/v1")
	{
		// User routes
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
	}
}

// @Security BearerAuth
func (a *API) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{"title": "Unauthorized", "detail": "Missing Authorization header"}}})
			return
		}

		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 || bearerToken[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{"title": "Unauthorized", "detail": "Invalid Authorization header format"}}})
			return
		}

		token := bearerToken[1]
		// TODO: Implement token validation logic here
		// For now, we'll just check if the token is not empty
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{"title": "Unauthorized", "detail": "Invalid token"}}})
			return
		}

		c.Next()
	}
}
