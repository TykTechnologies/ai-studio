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

	l := &analytics.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(body), maxBodySize),
		ResponseCode: statusCode,
	}

	analytics.RecordProxyLog(l)
	AnalyzeCompletionResponse(service, llm, app, response)
}

func AnalyzeStreamingResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, statusCode int, responses []byte, reqBody []byte, r *http.Request, chunks [][]byte) {
	llm, app, response, err := switches.AnalyzeStreamingResponse(llm, app, statusCode, responses, r, chunks)
	if err != nil {
		log.Printf("failed to analyze response: %v", err)
		return
	}

	l := &analytics.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(responses), maxBodySize),
		ResponseCode: statusCode,
	}

	analytics.RecordProxyLog(l)

	AnalyzeCompletionResponse(service, llm, app, response)
}

func AnalyzeCompletionResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse) {
	cpt := 0.0
	cpit := 0.0
	price, err := service.GetModelPriceByModelNameAndVendor(response.GetModel(), string(llm.Vendor))
	if err == nil {
		cpt = price.CPT
		cpit = price.CPIT
	}

	pt := response.GetPromptTokens()
	rt := response.GetResponseTokens()

	rec := &analytics.LLMChatRecord{
		Vendor:          string(llm.Vendor),
		PromptTokens:    response.GetPromptTokens(),
		ResponseTokens:  response.GetResponseTokens(),
		TotalTokens:     pt + rt,
		TimeStamp:       time.Now(),
		Choices:         response.GetChoiceCount(),
		ToolCalls:       response.GetToolCount(),
		AppID:           app.ID,
		UserID:          app.UserID,
		Cost:            (cpt * float64(rt)) + (cpit * float64(pt)),
		InteractionType: analytics.ProxyInteraction,
	}

	analytics.RecordChatRecord(rec)
}
