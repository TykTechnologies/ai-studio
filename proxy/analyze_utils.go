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

	rec := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Vendor:          string(llm.Vendor),
		PromptTokens:    pt,
		ResponseTokens:  rt,
		TotalTokens:     pt + rt,
		TimeStamp:       timestamp,
		Choices:         choices,
		ToolCalls:       tools,
		AppID:           app.ID,
		UserID:          app.UserID,
		Cost:            (cpt * float64(rt)) + (cpit * float64(pt)),
		InteractionType: models.ProxyInteraction,
	}

	analytics.RecordChatRecord(rec)
}
