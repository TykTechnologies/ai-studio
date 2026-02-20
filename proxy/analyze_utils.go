package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/andybalholm/brotli"
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

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(service, llm, app, response, r, time.Now())
}

func AnalyzeStreamingResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, statusCode int, responses []byte, reqBody []byte, r *http.Request, chunks [][]byte, timestamp time.Time, contentEncoding string) {
	decompressedResponses, err := decompressResponseBody(responses, contentEncoding)
	if err != nil {
		logger.Errorf("Failed to analyze streaming response: %v", err)
		return
	}

	llm, app, response, err := switches.AnalyzeStreamingResponse(llm, app, statusCode, decompressedResponses, r, chunks)
	if err != nil {
		logger.Errorf("Failed to analyze streaming response: %v", err)
		return
	}

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(decompressedResponses), maxBodySize),
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

	// Record the chat record with retries
	analytics.RecordChatRecord(rec)
	// time.Sleep(200 * time.Millisecond) // Removed: Unreliable fixed sleep. Test should handle waiting.

	// Budget analysis
	if s, ok := service.(*services.Service); ok && s.Budget != nil {
		s.Budget.AnalyzeBudgetUsage(app, llm)
	} else if budgetService, ok := service.(interface {
		GetBudgetService() services.BudgetService
	}); ok {
		if bs := budgetService.GetBudgetService(); bs != nil {
			bs.AnalyzeBudgetUsage(app, llm)
		}
	}
}

func decompressResponseBody(data []byte, contentEncoding string) ([]byte, error) {
	const bytesLimit = 10 * 1024 * 1024

	if len(data) == 0 || contentEncoding == "" {
		return data, nil
	}

	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer func() {
			if err := reader.Close(); err != nil {
				logger.Errorf("failed to close gzip reader: %v", err)
			}
		}()

		limitedReader := io.LimitedReader{R: reader, N: bytesLimit}

		decompressed, err := io.ReadAll(&limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip data: %v", err)
		}

		if limitedReader.N == 0 && len(decompressed) == bytesLimit {
			return nil, fmt.Errorf("decompressed data exceeds maximum allowed size of %d bytes", bytesLimit)
		}

		return decompressed, nil

	case "br", "brotli":
		limitedReader := io.LimitedReader{R: brotli.NewReader(bytes.NewReader(data)), N: bytesLimit}

		decompressed, err := io.ReadAll(&limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress brotli data: %v", err)
		}

		if limitedReader.N == 0 && len(decompressed) == bytesLimit {
			return nil, fmt.Errorf("decompressed data exceeds maximum allowed size of %d bytes", bytesLimit)
		}

		return decompressed, nil

	default:
		logger.Errorf("Decompression is not supported for %s, returning original data", contentEncoding)
		return data, nil
	}
}
