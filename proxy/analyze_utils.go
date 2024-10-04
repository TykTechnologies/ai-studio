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
		RequestBody:  string(reqBody),
		ResponseBody: string(body),
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
		RequestBody:  string(reqBody),
		ResponseBody: string(responses),
		ResponseCode: statusCode,
	}

	analytics.RecordProxyLog(l)

	AnalyzeCompletionResponse(service, llm, app, response)
}

func AnalyzeCompletionResponse(service services.ServiceInterface, llm *models.LLM, app *models.App, response models.ITokenResponse) {
	cpt := 0.0
	price, err := service.GetModelPriceByModelNameAndVendor(response.GetModel(), string(llm.Vendor))
	if err == nil {
		cpt = price.CPT
	}

	tt := response.GetPromptTokens() + response.GetResponseTokens()
	rec := &analytics.LLMChatRecord{
		Vendor:         string(llm.Vendor),
		PromptTokens:   response.GetPromptTokens(),
		ResponseTokens: response.GetResponseTokens(),
		TotalTokens:    tt,
		TimeStamp:      time.Now(),
		Choices:        response.GetChoiceCount(),
		ToolCalls:      response.GetToolCount(),
		AppID:          app.ID,
		UserID:         app.UserID,
		Cost:           cpt * float64(tt),
	}

	analytics.RecordChatRecord(rec)
}
