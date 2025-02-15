package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAnalyticsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{})
	require.NoError(t, err)

	return db
}

func setupAnalyticsTestAPI(db *gorm.DB) *API {
	gin.SetMode(gin.TestMode)
	return &API{service: &services.Service{DB: db}}
}

func TestGetCostAnalysis(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	api := setupAnalyticsTestAPI(db)

	// Insert test data
	now := time.Now()
	testData := []models.LLMChatRecord{
		{
			Vendor:          "openai",
			Cost:            10.0,
			Currency:        "USD",
			TimeStamp:       now,
			InteractionType: models.ChatInteraction,
		},
		{
			Vendor:          "openai",
			Cost:            20.0,
			Currency:        "USD",
			TimeStamp:       now,
			InteractionType: models.ProxyInteraction,
		},
	}
	for _, record := range testData {
		db.Create(&record)
	}

	tests := []struct {
		name            string
		interactionType string
		expectedCost    float64
	}{
		{
			name:            "Filter Chat Interactions",
			interactionType: string(models.ChatInteraction),
			expectedCost:    10.0,
		},
		{
			name:            "Filter Proxy Interactions",
			interactionType: string(models.ProxyInteraction),
			expectedCost:    20.0,
		},
		{
			name:            "No Filter",
			interactionType: "",
			expectedCost:    30.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set up request with query parameters
			req := httptest.NewRequest("GET", "/analytics/cost-analysis", nil)
			q := req.URL.Query()
			q.Add("start_date", now.AddDate(0, 0, -1).Format("2006-01-02"))
			q.Add("end_date", now.AddDate(0, 0, 1).Format("2006-01-02"))
			if tt.interactionType != "" {
				q.Add("interaction_type", tt.interactionType)
			}
			req.URL.RawQuery = q.Encode()
			c.Request = req

			api.getCostAnalysis(c)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]*models.ChartData
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Verify the total cost matches expected
			totalCost := 0.0
			for _, chartData := range response {
				for _, cost := range chartData.Data {
					totalCost += cost
				}
			}
			assert.InDelta(t, tt.expectedCost, totalCost, 0.001)
		})
	}
}

func TestGetMostUsedLLMModels(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	api := setupAnalyticsTestAPI(db)

	// Insert test data
	now := time.Now()
	testData := []models.LLMChatRecord{
		{
			Name:            "gpt-4",
			InteractionType: models.ChatInteraction,
			TimeStamp:       now,
		},
		{
			Name:            "gpt-4",
			InteractionType: models.ProxyInteraction,
			TimeStamp:       now,
		},
		{
			Name:            "gpt-4",
			InteractionType: models.ProxyInteraction,
			TimeStamp:       now,
		},
	}
	for _, record := range testData {
		db.Create(&record)
	}

	tests := []struct {
		name            string
		interactionType string
		expectedCount   float64
	}{
		{
			name:            "Filter Chat Interactions",
			interactionType: string(models.ChatInteraction),
			expectedCount:   1,
		},
		{
			name:            "Filter Proxy Interactions",
			interactionType: string(models.ProxyInteraction),
			expectedCount:   2,
		},
		{
			name:            "No Filter",
			interactionType: "",
			expectedCount:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("GET", "/analytics/most-used-llm-models", nil)
			q := req.URL.Query()
			q.Add("start_date", now.AddDate(0, 0, -1).Format("2006-01-02"))
			q.Add("end_date", now.AddDate(0, 0, 1).Format("2006-01-02"))
			if tt.interactionType != "" {
				q.Add("interaction_type", tt.interactionType)
			}
			req.URL.RawQuery = q.Encode()
			c.Request = req

			api.getMostUsedLLMModels(c)

			assert.Equal(t, http.StatusOK, w.Code)

			var response models.ChartData
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Len(t, response.Data, 1)
			assert.Equal(t, tt.expectedCount, response.Data[0])
		})
	}
}
