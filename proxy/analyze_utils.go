package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
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
	applyDecisionRequest(response, reqBody, r)

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    llm.Name,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(body), maxBodySize),
		ResponseCode: statusCode,
		RequestKind:  requestKindOf(response),
		ServiceTier:  serviceTierOf(response),
	}

	if llm.DontLogBodies {
		l.RequestBody = ""
		l.ResponseBody = ""
	}
	applyFailoverMarker(l, r.Context())

	// Use WithoutCancel to preserve trace context without lifecycle coupling,
	// since this function is called from goroutines after the HTTP response is sent.
	ctx := context.WithoutCancel(r.Context())
	if response == nil {
		// The vendor recognised a call that carries no usage (Anthropic
		// count_tokens): log it, without a chat record or budget usage.
		analytics.RecordExchange(ctx, l, nil)
		captureLLM(r, llm, app, statusCode, nil, reqBody, body, false)
		return
	}
	record := analyzeExchange(service, llm, app, response, ctx, r, l, time.Now())
	captureLLM(r, llm, app, statusCode, record, reqBody, body, false)
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
	applyDecisionRequest(response, reqBody, r)

	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    llm.Name,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(decompressedResponses), maxBodySize),
		ResponseCode: statusCode,
		RequestKind:  requestKindOf(response),
		ServiceTier:  serviceTierOf(response),
	}

	if llm.DontLogBodies {
		l.RequestBody = ""
		l.ResponseBody = ""
	}
	applyFailoverMarker(l, r.Context())

	// Use WithoutCancel to preserve trace context without lifecycle coupling,
	// since this function is called from goroutines after the HTTP response is sent.
	ctx := context.WithoutCancel(r.Context())
	if response == nil {
		// A stream that carries no usage here (an OpenAI background
		// response, accounted by the native reconciler): log it only.
		analytics.RecordExchange(ctx, l, nil)
		captureLLM(r, llm, app, statusCode, nil, reqBody, decompressedResponses, true)
		return
	}
	record := analyzeExchange(service, llm, app, response, ctx, r, l, timestamp)
	captureLLM(r, llm, app, statusCode, record, reqBody, decompressedResponses, true)
}

// analyzeExchange records a request's proxy log and chat record in one call
// (analytics.RecordExchange), then runs the steps that follow a chat record.
func analyzeExchange(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, ctx context.Context, r *http.Request, l *models.ProxyLog, timestamp time.Time) *models.LLMChatRecord {
	rec := buildChatRecord(service, llm, app, response, r, timestamp)
	analytics.RecordExchange(ctx, l, rec)
	afterChatRecord(service, llm, app, rec, ctx, r, timestamp)
	return rec
}

func AnalyzeCompletionResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, ctx context.Context, r *http.Request, timestamp time.Time) {
	rec := buildChatRecord(service, llm, app, response, r, timestamp)

	// Record the chat record with retries
	analytics.RecordChatRecord(ctx, rec)

	afterChatRecord(service, llm, app, rec, ctx, r, timestamp)
}

// buildChatRecord turns a parsed response into the request's chat record,
// priced from the model's price.
func buildChatRecord(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, r *http.Request, timestamp time.Time) *models.LLMChatRecord {
	return buildChatRecordFor(service, llm, app, response, requestedModel(r), timestamp)
}

// buildChatRecordFor is buildChatRecord without a request: requested is the
// model the caller asked for (the fallback name and price when the response
// names none, or an unpriced one). Usage recorded after the request that
// produced it has gone, such as an async batch's results, is built here.
// A response implementing models.IPriceAdjustedResponse has its cost scaled
// by its multiplier.
func buildChatRecordFor(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, requested string, timestamp time.Time) *models.LLMChatRecord {
	var pt, rt, choices, tools, cacheWriteTokens, cacheReadTokens int
	// Get model from response, fallback to the requested model if not available
	model := ""
	if response != nil {
		model = response.GetModel()
	}
	if model == "" {
		model = requested
	}

	if response != nil {
		pt = response.GetPromptTokens()
		rt = response.GetResponseTokens()
		choices = response.GetChoiceCount()
		tools = response.GetToolCount()
		cacheWriteTokens = response.GetCacheWritePromptTokens()
		cacheReadTokens = response.GetCacheReadPromptTokens()
	}

	// Get pricing information
	var cpt, cpit, cacheWritePT, cacheReadPT float64
	var currency string = "USD" // Default currency if no price found
	price, err := priceForExchange(service, model, requested, string(llm.Vendor))
	if err == nil && price != nil { // Check price != nil to avoid nil dereference
		cpt = price.CPT
		cpit = price.CPIT
		cacheWritePT = price.CacheWritePT
		cacheReadPT = price.CacheReadPT
		currency = price.Currency
	} else {
		log.Printf("Price not found for model: %s, vendor: %s", model, llm.Vendor)
	}

	// Subtract cached tokens from prompt tokens for vendors whose prompt token
	// count already includes them, so the cache tokens are not counted twice in
	// TotalTokens or in the cost below: Google's promptTokenCount includes
	// cachedContentTokenCount, and OpenAI's prompt_tokens includes
	// prompt_tokens_details.cached_tokens (and cache_write_tokens).
	pt = models.PromptTokensExcludingCache(string(llm.Vendor), pt, cacheWriteTokens, cacheReadTokens)

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
		// An unset (zero) cache price bills those tokens at the input price.
		Cost:            models.ChatRecordCost(string(llm.Vendor), cpt, cpit, cacheWritePT, cacheReadPT, pt, rt, cacheWriteTokens, cacheReadTokens),
		Currency:        currency, // Set the currency (defaults to USD if no price found)
		InteractionType: models.ProxyInteraction,
		RequestedModel:  requested,
	}
	multiplier := 1.0
	if pa, ok := response.(models.IPriceAdjustedResponse); ok {
		// The LLM's per-tier override (negotiated pricing) wins over the
		// vendor's multiplier.
		multiplier = llm.PriceMultipliers.Multiplier(pa.ServiceTier(), pa.PriceMultiplier())
		rec.Cost *= multiplier
		rec.ServiceTier = pa.ServiceTier()
	}
	rec.PriceMultiplier = &multiplier
	if dr, ok := response.(models.IDecisionResponse); ok {
		rec.RequestKind = models.DecisionRequest
		rec.DecisionQuestions = dr.GetDecisionQuestions()
		rec.DecisionRefusals = dr.GetDecisionRefusals()
	}
	return rec
}

// requestedModel is the model the caller asked for, as the model validation
// middleware read it from the request.
func requestedModel(r *http.Request) string {
	if r == nil {
		return ""
	}
	m, _ := r.Context().Value("model_name").(string)
	return m
}

// priceForExchange prices an exchange by the model the vendor reported and,
// when that model has no price, by the model the caller asked for. Vendors
// answer an alias with the model it resolved to (TypeSafe's jev-latest comes
// back as jev-1.13.0; OpenAI dates its snapshots), and an admin prices the
// name they configured. Without the fallback such calls were recorded at
// zero cost and never reached a budget. A miss is a zero price on both the
// Studio and the edge side (Studio stores one), so "no price" is all rates
// zero.
func priceForExchange(service services.ServiceInterface, reported, requested, vendor string) (*models.ModelPrice, error) {
	price, err := service.GetModelPriceByModelNameAndVendor(reported, vendor)
	if requested == "" || requested == reported || (err == nil && price != nil && !unpriced(price)) {
		return price, err
	}
	fallback, ferr := service.GetModelPriceByModelNameAndVendor(requested, vendor)
	if ferr == nil && fallback != nil && !unpriced(fallback) {
		return fallback, nil
	}
	return price, err
}

func unpriced(p *models.ModelPrice) bool {
	return p.CPT == 0 && p.CPIT == 0 && p.CacheWritePT == 0 && p.CacheReadPT == 0
}

// serviceTierOf is the vendor service tier an analysed response was billed
// at, when it says ("" for list-price traffic).
func serviceTierOf(response models.ITokenResponse) string {
	if pa, ok := response.(models.IPriceAdjustedResponse); ok {
		return pa.ServiceTier()
	}
	return ""
}

// requestKindOf is the request kind an analysed response belongs to.
func requestKindOf(response models.ITokenResponse) models.RequestKind {
	if _, ok := response.(models.IDecisionResponse); ok {
		return models.DecisionRequest
	}
	return models.GenerativeRequest
}

// applyDecisionRequest recounts a decision response's questions against the
// request that produced it: System One responses leave unanswered questions
// out, so only the request says how many were asked.
func applyDecisionRequest(response models.ITokenResponse, reqBody []byte, r *http.Request) {
	dr, ok := response.(*responses.DecisionResponse)
	if !ok || len(reqBody) == 0 {
		return
	}
	format, ok := decisions.FormatForPath(r.URL.Path)
	if !ok {
		return
	}
	dr.ApplyRequest(reqBody, format)
}

// afterChatRecord runs what follows a recorded chat record: the
// time-per-output-token metric and the budget analysis.
func afterChatRecord(service services.ServiceInterface, llm *models.LLM, app *models.App, rec *models.LLMChatRecord, ctx context.Context, r *http.Request, timestamp time.Time) {
	// time-per-output-token needs both the streaming landmarks and the completion
	// token count, and this is the only point where both are in hand. The timing
	// is absent on non-streaming requests, which correctly skips the metric.
	if elapsed, ok := streamTimingFrom(r.Context()).sinceFirstToken(timestamp); ok {
		metrics.ObserveTimePerOutputToken(ctx, string(llm.Vendor), rec.Name, elapsed.Seconds(), rec.ResponseTokens)
	}
	// time.Sleep(200 * time.Millisecond) // Removed: Unreliable fixed sleep. Test should handle waiting.

	analyzeBudget(budgetServiceOf(service), llm, app, rec)
}

// budgetServiceOf returns the budget service behind service, or nil.
func budgetServiceOf(service services.ServiceInterface) services.BudgetServiceInterface {
	if s, ok := service.(*services.Service); ok && s.Budget != nil {
		return s.Budget
	}
	if budgetService, ok := service.(interface {
		GetBudgetService() services.BudgetService
	}); ok {
		if bs := budgetService.GetBudgetService(); bs != nil {
			return bs
		}
	}
	return nil
}

// analyzeBudget runs what follows a recorded chat record on the budget side:
// the request's cost goes into the budget service's running totals (so the
// next CheckBudget counts it without waiting for the analytics write to reach
// the database), then the threshold analysis runs. rec may be nil.
func analyzeBudget(bs services.BudgetServiceInterface, llm *models.LLM, app *models.App, rec *models.LLMChatRecord) {
	if bs == nil {
		return
	}
	if sr, ok := bs.(budget.SpendRecorder); ok && rec != nil && rec.Cost > 0 {
		sr.RecordSpend(app, llm, rec.Cost/10000)
	}
	bs.AnalyzeBudgetUsage(app, llm)
}

func decompressResponseBody(data []byte, contentEncoding string) ([]byte, error) {
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

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip data: %v", err)
		}

		return decompressed, nil

	case "br", "brotli":
		decompressed, err := io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
		if err != nil {
			return nil, fmt.Errorf("failed to decompress brotli data: %v", err)
		}

		return decompressed, nil

	default:
		return nil, fmt.Errorf("decompression is not supported for %s", contentEncoding)
	}
}
