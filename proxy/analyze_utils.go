package proxy

import (
	"fmt"
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

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(service, llm, app, response, time.Now())
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
	AnalyzeCompletionResponse(service, llm, app, response, timestamp)
}

func AnalyzeCompletionResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse, timestamp time.Time) {
	var pt, rt, choices, tools int
	var model string

	if response != nil {
		pt = response.GetPromptTokens()
		rt = response.GetResponseTokens()
		choices = response.GetChoiceCount()
		tools = response.GetToolCount()
		model = response.GetModel()
	}

	cpt := 0.0
	cpit := 0.0
	price, err := service.GetModelPriceByModelNameAndVendor(model, string(llm.Vendor))
	if err == nil {
		cpt = price.CPT
		cpit = price.CPIT
	}

	// Use budget start date if set, otherwise use current time
	recordTime := timestamp
	if app.BudgetStartDate != nil {
		recordTime = *app.BudgetStartDate
	}
	if llm.BudgetStartDate != nil {
		recordTime = *llm.BudgetStartDate
	}

	rec := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Vendor:          string(llm.Vendor),
		PromptTokens:    pt,
		ResponseTokens:  rt,
		TotalTokens:     pt + rt,
		TimeStamp:       recordTime,
		Choices:         choices,
		ToolCalls:       tools,
		AppID:           app.ID,
		UserID:          app.UserID,
		Cost:            (cpt * float64(rt)) + (cpit * float64(pt)),
		InteractionType: models.ProxyInteraction,
	}

	// Record the chat record with retries
	for i := 0; i < 5; i++ {
		analytics.RecordChatRecord(rec)
		time.Sleep(50 * time.Millisecond) // Reduced from 500ms

		// Verify the record was committed
		if s, ok := service.(*services.Service); ok {
			var count int64
			if err := s.DB.Model(&models.LLMChatRecord{}).Where("app_id = ? AND llm_id = ? AND cost = ?", app.ID, llm.ID, rec.Cost).Count(&count).Error; err == nil && count > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // Reduced from 100ms
	}

	// Then analyze budget usage and send notifications if needed
	if s, ok := service.(*services.Service); ok && s.Budget != nil {
		// Wait for any pending operations to complete
		time.Sleep(100 * time.Millisecond) // Reduced from 1s

		// Analyze budget usage with retries
		for i := 0; i < 5; i++ {
			s.Budget.AnalyzeBudgetUsage(app, llm)
			time.Sleep(50 * time.Millisecond) // Reduced from 500ms

			// Verify notifications were created
			var count int64
			if err := s.DB.Model(&models.Notification{}).Where("notification_id LIKE ?", fmt.Sprintf("budget_app_%d_%%", app.ID)).Count(&count).Error; err == nil && count > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond) // Reduced from 100ms
		}
	}
}
