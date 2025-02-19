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

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.App{}, &models.LLM{})
	require.NoError(t, err)

	return db
}

func setupAnalyticsTestAPI(db *gorm.DB) (*API, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	api := &API{service: services.NewService(db)}
	router := gin.Default()
	return api, router
}

func TestGetCostAnalysis(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	api, router := setupAnalyticsTestAPI(db)
	router.GET("/analytics/cost-analysis", api.getCostAnalysis)

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

			router.ServeHTTP(w, req)

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

func TestGetBudgetUsage(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	api, router := setupAnalyticsTestAPI(db)
	router.GET("/analytics/budget-usage", api.getBudgetUsage)

	// Create test app with budget
	budget := 100.0
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	app := &models.App{
		Name:            "Test App",
		MonthlyBudget:   &budget,
		BudgetStartDate: &startOfMonth,
	}
	db.Create(app)

	// Create test LLM with budget
	llmBudget := 200.0
	llm := &models.LLM{
		Name:            "Test LLM",
		MonthlyBudget:   &llmBudget,
		BudgetStartDate: &startOfMonth,
	}
	db.Create(llm)

	// Create test records
	records := []models.LLMChatRecord{
		{
			AppID:     app.ID,
			LLMID:     llm.ID,
			Cost:      50.0,
			TimeStamp: startOfMonth.Add(24 * time.Hour),
		},
	}
	for _, record := range records {
		db.Create(&record)
	}

	// Call getBudgetUsage
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/analytics/budget-usage", nil)

	router.ServeHTTP(w, req)

	// Verify successful response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Should have 2 entries (1 app + 1 LLM)
	assert.Len(t, response, 2)

	// Find app and LLM entries
	var appEntry, llmEntry map[string]interface{}
	for _, entry := range response {
		if entry["entity_type"] == "App" {
			appEntry = entry
		} else if entry["entity_type"] == "LLM" {
			llmEntry = entry
		}
	}

	// Verify app budget usage
	assert.NotNil(t, appEntry)
	assert.Equal(t, "Test App", appEntry["name"])
	assert.Equal(t, 50.0, appEntry["spent"])
	assert.Equal(t, 50.0, appEntry["usage"]) // 50/100 * 100

	// Verify LLM budget usage
	assert.NotNil(t, llmEntry)
	assert.Equal(t, "Test LLM", llmEntry["name"])
	assert.Equal(t, 50.0, llmEntry["spent"])
	assert.Equal(t, 25.0, llmEntry["usage"]) // 50/200 * 100
}

func TestGetMostUsedLLMModels(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	api, router := setupAnalyticsTestAPI(db)
	router.GET("/analytics/most-used-llm-models", api.getMostUsedLLMModels)

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

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response models.ChartData
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Len(t, response.Data, 1)
			assert.Equal(t, tt.expectedCount, response.Data[0])
		})
	}
}
