// Package aigateway provides interfaces for gateway service implementations
package aigateway

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// GatewayServiceInterface defines the core operations needed by the gateway proxy.
// This interface abstracts the data layer, allowing implementations to use databases,
// files, APIs, or any other backend while providing the same functionality.
type GatewayServiceInterface interface {
	// Configuration loading (read-only)
	GetActiveLLMs() ([]models.LLM, error)
	GetActiveDatasources() ([]models.Datasource, error)
	GetToolBySlug(slug string) (*models.Tool, error)

	// Authentication & Authorization (read-only)
	GetCredentialBySecret(secret string) (*models.Credential, error)
	GetAppByCredentialID(credID uint) (*models.App, error)
	GetUserByID(id uint) (*models.User, error)

	// OAuth Authentication (read-only)
	GetValidAccessTokenByToken(token string) (*models.AccessToken, error)
	GetOAuthClient(clientID string) (*models.OAuthClient, error)

	// Pricing (read-only)
	GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error)

	// Tool operations
	CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error)
}

// GatewayBudgetServiceInterface defines budget operations needed by the gateway proxy.
// This interface handles budget validation and usage tracking, allowing for different
// budget backends (database, API, in-memory, etc.).
type GatewayBudgetServiceInterface interface {
	// CheckBudget verifies if a request would exceed either App or LLM budget.
	// Returns app usage percentage, llm usage percentage, and error if budget exceeded.
	CheckBudget(app *models.App, llm *models.LLM) (appUsage, llmUsage float64, err error)

	// AnalyzeBudgetUsage analyzes current budget usage and sends notifications if thresholds are reached.
	// This runs in the background and doesn't block the main request flow.
	AnalyzeBudgetUsage(app *models.App, llm *models.LLM)
}

// AnalyticsHandler defines the interface for analytics implementations
// This interface allows custom analytics backends (HTTP APIs, message queues, etc.)
type AnalyticsHandler interface {
	// RecordChatRecord records LLM chat/proxy usage
	RecordChatRecord(record *models.LLMChatRecord)

	// RecordProxyLog records proxy request/response logs
	RecordProxyLog(log *models.ProxyLog)

	// RecordToolCall records tool call execution
	RecordToolCall(name string, timestamp time.Time, execTime int, toolID uint)

	// SetAsGlobalHandler sets this handler as the global analytics handler
	SetAsGlobalHandler()
}
