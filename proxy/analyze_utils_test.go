package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

type MockService struct {
	mock.Mock
	services.ServiceInterface
}

func (m *MockService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	args := m.Called(modelName, vendor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModelPrice), args.Error(1)
}

type MockTokenResponse struct {
	models.ITokenResponse
	model            string
	promptTokens     int
	responseTokens   int
	choiceCount      int
	toolCount        int
	cacheWriteTokens int
	cacheReadTokens  int
}

func (m *MockTokenResponse) GetModel() string {
	return m.model
}

func (m *MockTokenResponse) GetPromptTokens() int {
	return m.promptTokens
}

func (m *MockTokenResponse) GetResponseTokens() int {
	return m.responseTokens
}

func (m *MockTokenResponse) GetChoiceCount() int {
	return m.choiceCount
}

func (m *MockTokenResponse) GetToolCount() int {
	return m.toolCount
}

func (m *MockTokenResponse) GetCacheWritePromptTokens() int {
	return m.cacheWriteTokens
}

func (m *MockTokenResponse) GetCacheReadPromptTokens() int {
	return m.cacheReadTokens
}

func TestAnalyzeCompletionResponse_Currency(t *testing.T) {
	tests := []struct {
		name             string
		price            *models.ModelPrice
		expectedCurrency string
	}{
		{
			name: "with price model",
			price: &models.ModelPrice{
				ModelName: "test-model",
				Vendor:    "test-vendor",
				CPT:       0.001,
				CPIT:      0.002,
				Currency:  "EUR",
			},
			expectedCurrency: "EUR",
		},
		{
			name:             "without price model",
			price:            nil,
			expectedCurrency: "USD", // Default currency
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := new(MockService)
			mockService.On("GetModelPriceByModelNameAndVendor", "test-model", "test-vendor").Return(tt.price, nil)

			response := &MockTokenResponse{
				model:            "test-model",
				promptTokens:     100,
				responseTokens:   50,
				choiceCount:      1,
				toolCount:        0,
				cacheWriteTokens: 0,
				cacheReadTokens:  0,
			}

			llm := &models.LLM{
				ID:     1,
				Vendor: models.VendorType("test-vendor"),
			}

			app := &models.App{
				ID:     1,
				UserID: 1,
			}

			var recordedCurrency string
			analytics.RecordChatRecord = func(rec *models.LLMChatRecord) {
				recordedCurrency = rec.Currency
			}

			AnalyzeCompletionResponse(mockService, llm, app, response, time.Now())

			assert.Equal(t, tt.expectedCurrency, recordedCurrency)
			mockService.AssertExpectations(t)
		})
	}
}
