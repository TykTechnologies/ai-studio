package proxy

import (
	"log"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

const maxBodySize = 65535 // Maximum size for TEXT column (64KB)

func truncateString(s string, maxSize int) string {
	if len(s) <= maxSize {
		return s
	}
	return s[:maxSize]
}

func AnalyzeResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, statusCode int, body []byte, reqBody []byte, r *http.Request) {
	llm, app, response, err := switches.AnalyzeResponse(llm, app, statusCode, body, r)
	if err != nil {
		log.Printf("failed to analyze response: %v", err)
		return
	}

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(body), maxBodySize),
		ResponseCode: statusCode,
	}

	log.Printf("🔍 ANALYTICS DEBUG: Creating ProxyLog - AppID=%d, UserID=%d (from app.UserID)", app.ID, app.UserID)
	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(service, llm, app, response, r, time.Now())
}

func AnalyzeStreamingResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, statusCode int, responses []byte, reqBody []byte, r *http.Request, chunks [][]byte, timestamp time.Time) {
	llm, app, response, err := switches.AnalyzeStreamingResponse(llm, app, statusCode, responses, r, chunks)
	if err != nil {
		log.Printf("failed to analyze response: %v", err)
		return
	}

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(responses), maxBodySize),
		ResponseCode: statusCode,
	}

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(service, llm, app, response, r, timestamp)
}

func AnalyzeCompletionResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, r *http.Request, timestamp time.Time) {
	var pt, rt, choices, tools int
	// Get model from response, fallback to context if not available
	model := ""
	if response != nil {
		model = response.GetModel()
	}

	if model == "" {
		if modelFromCtx := r.Context().Value("model_name"); modelFromCtx != nil {
			if modelStr, ok := modelFromCtx.(string); ok {
				model = modelStr
			}
		}
	}

	if response != nil {
		pt = response.GetPromptTokens()
		rt = response.GetResponseTokens()
		choices = response.GetChoiceCount()
		tools = response.GetToolCount()
	}

	// Get pricing information
	var cpt, cpit, cacheWritePT, cacheReadPT float64
	var currency string = "USD" // Default currency if no price found
	price, err := service.GetModelPriceByModelNameAndVendor(model, string(llm.Vendor))
	if err == nil && price != nil { // Check price != nil to avoid nil dereference
		cpt = price.CPT
		cpit = price.CPIT
		cacheWritePT = price.CacheWritePT
		cacheReadPT = price.CacheReadPT
		currency = price.Currency
	} else {
		log.Printf("Price not found for model: %s, vendor: %s", model, llm.Vendor)
	}

	// Get cache token counts
	cacheWriteTokens := response.GetCacheWritePromptTokens()
	cacheReadTokens := response.GetCacheReadPromptTokens()

	// Use actual timestamp for the record, not budget start dates
	rec := &models.LLMChatRecord{
		LLMID:                  llm.ID,
		Name:                   model, // Set the model name from the response
		Vendor:                 string(llm.Vendor),
		PromptTokens:           pt,
		ResponseTokens:         rt,
		CacheWritePromptTokens: cacheWriteTokens,
		CacheReadPromptTokens:  cacheReadTokens,
		TotalTokens:            pt + rt + cacheWriteTokens + cacheReadTokens,
		TimeStamp:              timestamp,
		Choices:                choices,
		ToolCalls:              tools,
		AppID:                  app.ID,
		UserID:                 app.UserID,
		Cost: ((cpt * float64(rt)) +
			(cpit * float64(pt)) +
			(cacheWritePT * float64(cacheWriteTokens)) +
			(cacheReadPT * float64(cacheReadTokens))) * 10000,
		Currency:        currency, // Set the currency (defaults to USD if no price found)
		InteractionType: models.ProxyInteraction,
	}

	log.Printf("🔍 ANALYTICS DEBUG: Creating LLMChatRecord - AppID=%d, UserID=%d (from app.UserID)", app.ID, app.UserID)
	// Record the chat record with retries
	analytics.RecordChatRecord(rec)
	// time.Sleep(200 * time.Millisecond) // Removed: Unreliable fixed sleep. Test should handle waiting.

	// Budget analysis
	if s, ok := service.(*services.Service); ok && s.Budget != nil {
		s.Budget.AnalyzeBudgetUsage(app, llm)
	} else if budgetService, ok := service.(interface {
		GetBudgetService() *services.BudgetService
	}); ok {
		if bs := budgetService.GetBudgetService(); bs != nil {
			bs.AnalyzeBudgetUsage(app, llm)
		}
	}
}
