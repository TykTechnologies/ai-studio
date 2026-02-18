package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

func TestTokenAnalytics_NotStreamResponse(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	var upstreamCalled bool
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		jsonResponse := `{
			"candidates": [
					{
							"content": {
									"parts": [
											{
													"text": "Hello there! How can I help you today?"
											}
									],
									"role": "model"
							},
							"finishReason": "STOP",
							"index": 0
					}
			],
			"usageMetadata": {
					"promptTokenCount": 3,
					"candidatesTokenCount": 10,
					"totalTokenCount": 36,
					"promptTokensDetails": [
							{
									"modality": "TEXT",
									"tokenCount": 3
							}
					],
					"thoughtsTokenCount": 23
			},
			"modelVersion": "gemini-2.5-flash",
			"responseId": "GSOUadPREf-Z28oPlc62cA"
	}`
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)

		writeGzippedResponse(t, w, []byte(jsonResponse))
	}))
	defer mockUpstream.Close()

	proxy, llm, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gemini-2.5-flash",
		Vendor:    string(models.GOOGLEAI),
		CPT:       0.0000025,
		CPIT:      0.0000003,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model":"gemini-2.5-flash", "contents": [{"role":"user","parts":[{"text":"Hello!"}]}]}`
	resp := sendProxyRequest(t, srv.URL, "gemini-api", secret, reqBody, false)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Hello there!")

	assert.True(t, upstreamCalled, "Mock upstream server was not called")

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, "gemini-2.5-flash", record.Name)
	assert.Equal(t, string(models.GOOGLEAI), record.Vendor)
	assert.Equal(t, 3, record.PromptTokens)
	assert.Equal(t, 33, record.ResponseTokens)
	assert.Equal(t, 36, record.TotalTokens)
	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, models.ProxyInteraction, record.InteractionType)
	assert.Equal(t, "USD", record.Currency)
	assert.InDelta(t, 0.834, record.Cost, 0.01)

	var proxyLog models.ProxyLog
	require.NoError(t, db.First(&proxyLog).Error)

	assert.Equal(t, uint(1), proxyLog.AppID)
	assert.Equal(t, http.StatusOK, proxyLog.ResponseCode)
	assert.Equal(t, string(models.GOOGLEAI), proxyLog.Vendor)
	assert.Contains(t, proxyLog.ResponseBody, "Hello there")
}

// Verifies that when the upstream LLM returns malformed JSON,
// the proxy still returns a response to the client but
// does NOT create an LLMChatRecord or ProxyLog, because AnalyzeResponse fails
// on the JSON unmarshal and returns early before recording anything.
func TestTokenAnalytics_MalformedJSON_NoRecord(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse := []byte(`{"id": "chatcmpl-bad", "usage": invalid}`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		writeGzippedResponse(t, w, jsonResponse)
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model":"gemini-2.5-flash", "contents": [{"role":"user","parts":[{"text":"Hello!"}]}]}`
	resp := sendProxyRequest(t, srv.URL, "gemini-api", secret, reqBody, false)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitUntilIdle(t, db)

	// No LLMChatRecord should be created because AnalyzeResponse fails on unmarshal
	var chatCount int64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error)
	assert.Equal(t, int64(0), chatCount, "no LLMChatRecord should exist for malformed JSON response")

	// No ProxyLog either — AnalyzeResponse returns early on unmarshal error,
	// before the ProxyLog is recorded. This is the current system behavior.
	var logCount int64
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&logCount).Error)
	assert.Equal(t, int64(0), logCount, "no ProxyLog should exist when response analysis fails")
}

func TestTokenAnalytics_Upstream500(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "Internal server error", "type": "server_error"}}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gemini-2.5-flash",
		Vendor:    string(models.GOOGLEAI),
		CPT:       0.0000025,
		CPIT:      0.0000003,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model":"gemini-2.5-flash", "contents": [{"role":"user","parts":[{"text":"Hello!"}]}]}`
	resp := sendProxyRequest(t, srv.URL, "gemini-api", secret, reqBody, false)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	waitUntilIdle(t, db)
	waitForProxyLog(t, db, 1, http.StatusInternalServerError)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("response_code = ?", http.StatusInternalServerError).First(&proxyLog).Error)
	assert.Equal(t, uint(1), proxyLog.AppID)
	assert.Equal(t, http.StatusInternalServerError, proxyLog.ResponseCode)
	assert.Equal(t, string(models.GOOGLEAI), proxyLog.Vendor)

	// The record is created with empty tokens values
	var chatCount int64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error)
	require.Equal(t, int64(chatCount), chatCount)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)
	assert.Equal(t, 0, record.PromptTokens, "prompt tokens should be zero for error response")
	assert.Equal(t, 0, record.ResponseTokens, "response tokens should be zero for error response")
	assert.Equal(t, 0, record.TotalTokens, "total tokens should be zero for error response")
	assert.InDelta(t, 0.0, record.Cost, 0.01, "cost should be zero for error response")
}

// Verifies that when no ModelPrice exists for the model, the system still records analytics but with zero cost.
func TestTokenAnalytics_NoPriceRecord(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse := []byte(`{
			"candidates": [
					{
							"content": {
									"parts": [
											{
													"text": "Hello there! How can I help you today?"
											}
									],
									"role": "model"
							},
							"finishReason": "STOP",
							"index": 0
					}
			],
			"usageMetadata": {
					"promptTokenCount": 3,
					"candidatesTokenCount": 10,
					"totalTokenCount": 36,
					"promptTokensDetails": [
							{
									"modality": "TEXT",
									"tokenCount": 3
							}
					],
					"thoughtsTokenCount": 23
			},
			"modelVersion": "gemini-2.5-flash",
			"responseId": "GSOUadPREf-Z28oPlc62cA"
	}`)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)

		writeGzippedResponse(t, w, jsonResponse)
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model":"gemini-2.5-flash", "contents": [{"role":"user","parts":[{"text":"Hello!"}]}]}`
	resp := sendProxyRequest(t, srv.URL, "gemini-api", secret, reqBody, false)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	// Tokens are recorded correctly even without a price
	assert.Equal(t, 3, record.PromptTokens)
	assert.Equal(t, 33, record.ResponseTokens)
	assert.Equal(t, 36, record.TotalTokens)

	// Cost should be zero because auto-created price has zero rates
	assert.InDelta(t, 0.0, record.Cost, 0.01)
}

func TestTokenAnalytics_StreamingRequest(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse := `[
    {
        "candidates": [
            {
                "content": {
                    "parts": [
                        {
                            "text": "The morning sun cast long shadows across the dew-kissed lawn, hinting at the warmth to come. A lone bird chirped melodically from a nearby branch, its song a"
                        }
                    ],
                    "role": "model"
                },
                "index": 0
            }
        ],
        "usageMetadata": {
            "promptTokenCount": 8,
            "candidatesTokenCount": 36,
            "totalTokenCount": 220,
            "promptTokensDetails": [
                {
                    "modality": "TEXT",
                    "tokenCount": 8
                }
            ],
            "thoughtsTokenCount": 176
        },
        "modelVersion": "gemini-2.5-flash",
        "responseId": "q3iUaaafIPjwxN8P54mE4QM"
    },
    {
        "candidates": [
            {
                "content": {
                    "parts": [
                        {
                            "text": " gentle start to the day."
                        }
                    ],
                    "role": "model"
                },
                "finishReason": "STOP",
                "index": 0
            }
        ],
        "usageMetadata": {
            "promptTokenCount": 8,
            "candidatesTokenCount": 43,
            "totalTokenCount": 227,
            "promptTokensDetails": [
                {
                    "modality": "TEXT",
                    "tokenCount": 8
                }
            ],
            "thoughtsTokenCount": 176
        },
        "modelVersion": "gemini-2.5-flash",
        "responseId": "q3iUaaafIPjwxN8P54mE4QM"
    }
]`

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		writeGzippedResponse(t, w, []byte(jsonResponse))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gemini-2.5-flash",
		Vendor:    string(models.GOOGLEAI),
		CPT:       0.0000025,
		CPIT:      0.0000003,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model":"gemini-2.5-flash", "contents": [{"role":"user","parts":[{"text":"Hello!"}]}]}`
	resp := sendProxyRequest(t, srv.URL, "gemini-api", secret, reqBody, true)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, 8, record.PromptTokens)
	assert.Equal(t, 219, record.ResponseTokens)
	assert.InDelta(t, 5.49, record.Cost, 0.01)
}

func setupIntegrationProxy(t *testing.T, db *gorm.DB, mockURL string) (*Proxy, *models.LLM, *models.App, string) {
	t.Helper()

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	user := &models.User{ID: 1, Email: "random@example.com"}
	require.NoError(t, db.Create(user).Error)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "gemini-api",
		Vendor:       models.GOOGLEAI,
		DefaultModel: "gemini-1.5-flash",
		Active:       true,
		APIEndpoint:  mockURL,
		APIKey:       "api-key",
	}
	require.NoError(t, db.Create(llm).Error)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "Test App",
		UserID: user.ID,
	}
	require.NoError(t, db.Create(app).Error)

	cred := &models.Credential{
		Model:  gorm.Model{ID: 1},
		Secret: "secret",
		Active: true,
	}
	require.NoError(t, db.Create(cred).Error)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*llm}
	require.NoError(t, db.Save(app).Error)

	require.NoError(t, proxy.loadResources())

	return proxy, llm, app, cred.Secret
}

func startProxyServer(t *testing.T, proxy *Proxy) *httptest.Server {
	t.Helper()

	r := mux.NewRouter()
	finalHandler := proxy.credValidator.Middleware(
		proxy.streamDetectionMiddleware(
			http.HandlerFunc(proxy.handleUnifiedLLMRequest),
		),
	)
	r.Handle("/llm/call/{llmSlug}/{rest:.*}", finalHandler)

	return httptest.NewServer(r)
}

func sendProxyRequest(t *testing.T, proxyURL, llmSlug, secret, reqBody string, isStreaming bool) *http.Response {
	t.Helper()

	url := fmt.Sprintf("%s/llm/call/%s/v1/models/gemini-2.5-flash:generateContent", proxyURL, llmSlug)
	if isStreaming {
		url = fmt.Sprintf("%s/llm/call/%s/v1/models/gemini-2.5-flash:streamGenerateContent", proxyURL, llmSlug)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(reqBody))
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}
