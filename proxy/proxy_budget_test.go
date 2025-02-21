package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestBudgetCheck(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	var err error

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)

	// Clear the budget service cache before starting
	budgetService.ClearCache()

	// Use fixed time for deterministic testing in local timezone
	loc := time.Now().Location()
	now := time.Date(2025, 2, 16, 10, 42, 13, 0, loc)
	startOfMonth := time.Date(2025, 2, 1, 0, 0, 0, 0, loc)

	// Create test LLM with budget
	monthlyBudget := 100.0
	llm := &models.LLM{
		Model:           gorm.Model{ID: 1},
		Name:            "TestLLM",
		Vendor:          models.MOCK_VENDOR,
		MonthlyBudget:   &monthlyBudget,
		DefaultModel:    "test-model",
		Active:          true,
		APIEndpoint:     "http://mock-api.example.com",
		BudgetStartDate: &startOfMonth,
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Force reload resources
	err = proxy.loadResources()
	require.NoError(t, err)

	// Setup mock upstream server
	requestCount := 0
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Copy request body to response
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Increment request count
		requestCount++

		// For non-streaming requests, send complete response
		if !strings.Contains(r.URL.Path, "/stream/") {
			// Send non-streaming response
			response := map[string]interface{}{
				"id":     "mock-123",
				"object": "chat.completion",
				"model":  "test-model",
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"content": "Hello world!",
						},
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     5000,
					"completion_tokens": 10000,
					"total_tokens":      15000,
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// For streaming requests, send chunks
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Send some content chunks
		chunks := []string{
			`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}`,
			`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" world"}}]}`,
			`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"!"}}]}`,
			// Final chunk with usage info in the same format as non-streaming response
			`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello world!"}}],"usage":{"prompt_tokens":5000,"completion_tokens":10000,"total_tokens":15000}}`,
		}

		for _, chunk := range chunks {
			// Write each chunk as a complete line
			_, _ = w.Write([]byte(chunk + "\n"))
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer mockUpstream.Close()

	// Update LLM to use mock server
	llm.APIEndpoint = mockUpstream.URL
	err = db.Save(llm).Error
	require.NoError(t, err)

	// Register mock validator
	proxy.credValidator.RegisterValidator(string(models.MOCK_VENDOR), func(r *http.Request) (string, error) {
		token := r.Header.Get("Authorization")
		return strings.TrimPrefix(token, "Bearer "), nil
	})

	// Create test user and admin
	user := &models.User{
		Model: gorm.Model{ID: 1},
		Email: "test@example.com",
	}
	admin := &models.User{
		Model:   gorm.Model{ID: 2},
		Email:   "admin@example.com",
		IsAdmin: true,
	}
	err = db.Create(user).Error
	require.NoError(t, err)
	err = db.Create(admin).Error
	require.NoError(t, err)

	// Create model price for test model
	modelPrice := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002, // Cost per response token
		CPIT:      0.001, // Cost per prompt token
		Currency:  "USD",
	}
	err = db.Create(modelPrice).Error
	require.NoError(t, err)

	// Verify model price was created correctly
	var checkPrice models.ModelPrice
	err = db.Where("model_name = ? AND vendor = ?", "test-model", string(models.MOCK_VENDOR)).First(&checkPrice).Error
	require.NoError(t, err)
	require.InDelta(t, 0.002, checkPrice.CPT, 0.0001, "Model price CPT should be 0.002")
	require.InDelta(t, 0.001, checkPrice.CPIT, 0.0001, "Model price CPIT should be 0.001")

	// Create test app with budget = 50 and start date
	appBudget := 50.0
	app := &models.App{
		Model:           gorm.Model{ID: 1},
		Name:            "TestApp",
		MonthlyBudget:   &appBudget,
		UserID:          user.ID,
		BudgetStartDate: &startOfMonth,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create credential for auth
	cred := &models.Credential{
		Model:  gorm.Model{ID: 1},
		Secret: "valid-token",
		Active: true,
	}
	err = db.Create(cred).Error
	require.NoError(t, err)

	// Associate app with credential
	app.CredentialID = cred.ID
	err = db.Save(app).Error
	require.NoError(t, err)

	// Many-to-many
	err = app.AddLLM(db, llm)
	require.NoError(t, err)

	// Reload resources
	err = proxy.loadResources()
	require.NoError(t, err)

	// Setup test server
	r := mux.NewRouter()
	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		newReq := r.Clone(r.Context())
		newReq.Body = io.NopCloser(bytes.NewBuffer(body))
		newReq.ContentLength = int64(len(body))

		proxy.handleLLMRequest(w, newReq)
	}).Methods("POST")
	r.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		newReq := r.Clone(r.Context())
		newReq.Body = io.NopCloser(bytes.NewBuffer(body))
		newReq.ContentLength = int64(len(body))

		proxy.handleStreamingLLMRequest(w, newReq)
	}).Methods("POST")
	srv := httptest.NewServer(proxy.credValidator.Middleware(r))
	defer srv.Close()

	// Helper to create a request
	makeRequest := func(streaming bool) *http.Request {
		reqBody := []byte(`{"prompt": "Hello"}`)
		endpoint := "/llm/rest/"
		if streaming {
			endpoint = "/llm/stream/"
		}
		req, _ := http.NewRequest("POST", srv.URL+endpoint+"testllm/v1/test", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(reqBody)))
		// Insert 'app' into context manually for test
		req = req.WithContext(context.WithValue(req.Context(), "app", app))
		return req
	}

	t.Run("Non-streaming request", func(t *testing.T) {
		// First request should cost 25.0 (5000 * 0.001 + 10000 * 0.002)
		resp, err := http.DefaultClient.Do(makeRequest(false))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Wait for analytics to process the request
		waitForAnalytics(t, db, 1)
		waitUntilIdle(t, db)
		record := waitForRecordWithCost(t, db)
		require.NotNil(t, record)

		// Wait for spending to be updated
		waitForSpendingUpdate(t, budgetService, app.ID, llm.ID, startOfMonth, record.TimeStamp.Add(time.Second), record.Cost)

		// Verify spending
		waitUntilIdle(t, db)
		spent, err := budgetService.GetMonthlySpending(app.ID, startOfMonth, now)
		require.NoError(t, err)
		assert.InDelta(t, 25.0, spent, 0.1, "App spending should be 25.0")

		llmSpent, err := budgetService.GetLLMMonthlySpending(llm.ID, startOfMonth, now)
		require.NoError(t, err)
		assert.InDelta(t, 25.0, llmSpent, 0.1, "LLM spending should be 25.0")
	})

	// Clear cache before next test
	budgetService.ClearCache()

	t.Run("Streaming request", func(t *testing.T) {
		// Second request should also cost 25.0
		resp, err := http.DefaultClient.Do(makeRequest(true))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Wait for analytics to process the request
		waitForAnalytics(t, db, 2)
		waitUntilIdle(t, db)
		record := waitForRecordWithCost(t, db)
		require.NotNil(t, record)

		// Clear cache and wait for spending to be updated
		budgetService.ClearCache()
		waitForSpendingUpdate(t, budgetService, app.ID, llm.ID, startOfMonth, record.TimeStamp.Add(time.Second), 50.0)
	})

	t.Run("Budget exceeded", func(t *testing.T) {
		// Wait for analytics and budget analysis to complete
		waitForAnalytics(t, db, 2)
		waitUntilIdle(t, db)
		time.Sleep(1 * time.Second) // Give more time for budget analysis and notifications

		// Clear cache to ensure fresh data
		budgetService.ClearCache()

		// Analyze budget usage again to ensure notifications are created
		budgetService.AnalyzeBudgetUsage(app, llm)
		time.Sleep(500 * time.Millisecond)

		// Wait for notifications to be created
		var notification models.Notification
		for i := 0; i < 10; i++ {
			monthOffset := int(startOfMonth.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
			err := db.Where("notification_id LIKE ? AND sent_at >= ?",
				fmt.Sprintf("budget_app_%d_%d_%d_%d_%%",
					app.ID,
					monthOffset,
					int(*app.MonthlyBudget),
					100),
				startOfMonth).First(&notification).Error
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		require.NoError(t, err, "Failed to find budget notification")

		// Third request should fail (would exceed 50.0 budget)
		resp, err := http.DefaultClient.Do(makeRequest(false))
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		var errorResp ErrorResponse
		err = json.Unmarshal(body, &errorResp)
		assert.NoError(t, err)
		assert.Contains(t, errorResp.Message, "Budget limit exceeded")
	})

	t.Run("No budget limits", func(t *testing.T) {
		// Wait for analytics to complete before modifying DB
		waitUntilIdle(t, db)

		// App with nil budget => not blocked
		app.MonthlyBudget = nil
		err = db.Save(app).Error
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(makeRequest(false))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		waitForAnalytics(t, db, 3)
		waitUntilIdle(t, db)

		// LLM with nil budget => not blocked
		llm.MonthlyBudget = nil
		err = db.Save(llm).Error
		require.NoError(t, err)

		resp, err = http.DefaultClient.Do(makeRequest(false))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Budget periods", func(t *testing.T) {
		// Wait for any pending operations to complete
		waitUntilIdle(t, db)
		time.Sleep(2 * time.Second)

		// Clear cache and all records before starting
		budgetService.ClearCache()
		err = db.Where("1 = 1").Delete(&models.Notification{}).Error
		require.NoError(t, err)
		err = db.Where("1 = 1").Delete(&models.LLMChatRecord{}).Error
		require.NoError(t, err)

		// Set budget start date to January 15th
		budgetStart := time.Date(2025, 1, 15, 0, 0, 0, 0, loc)

		// Update app and llm in a transaction
		err = db.Transaction(func(tx *gorm.DB) error {
			// Reset budgets
			appBudget := 50.0
			llmBudget := 100.0

			// Update app
			if err := tx.Model(&models.App{}).Where("id = ?", app.ID).Updates(map[string]interface{}{
				"monthly_budget":    appBudget,
				"budget_start_date": budgetStart,
			}).Error; err != nil {
				return err
			}

			// Update llm
			if err := tx.Model(&models.LLM{}).Where("id = ?", llm.ID).Updates(map[string]interface{}{
				"monthly_budget":    llmBudget,
				"budget_start_date": budgetStart,
			}).Error; err != nil {
				return err
			}

			return nil
		})
		require.NoError(t, err)

		// Reload app and llm
		err = db.First(&app, app.ID).Error
		require.NoError(t, err)
		err = db.First(&llm, llm.ID).Error
		require.NoError(t, err)

		// Create records for past period (Jan 15 - Feb 14)
		pastRecord1 := &models.LLMChatRecord{
			LLMID:           llm.ID,
			Vendor:          string(llm.Vendor),
			PromptTokens:    5000,
			ResponseTokens:  10000,
			TotalTokens:     15000,
			TimeStamp:       time.Date(2025, 1, 20, 10, 0, 0, 0, loc),
			AppID:           app.ID,
			UserID:          app.UserID,
			Cost:            25.0,
			InteractionType: models.ProxyInteraction,
		}
		err = db.Create(pastRecord1).Error
		require.NoError(t, err)

		pastRecord2 := &models.LLMChatRecord{
			LLMID:           llm.ID,
			Vendor:          string(llm.Vendor),
			PromptTokens:    5000,
			ResponseTokens:  10000,
			TotalTokens:     15000,
			TimeStamp:       time.Date(2025, 2, 1, 10, 0, 0, 0, loc),
			AppID:           app.ID,
			UserID:          app.UserID,
			Cost:            25.0,
			InteractionType: models.ProxyInteraction,
		}
		err = db.Create(pastRecord2).Error
		require.NoError(t, err)

		// Create notification for past period (Jan 15 - Feb 14)
		pastMonthOffset := int(budgetStart.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)
		pastNotification := &models.Notification{
			NotificationID: fmt.Sprintf("budget_app_%d_%d_%d_%d_owner",
				app.ID,
				pastMonthOffset,
				int(*app.MonthlyBudget),
				100),
			SentAt: time.Date(2025, 1, 20, 10, 0, 0, 0, loc), // Within the past period
			Type:   "budget_alert",
			Title:  "Budget Alert",
			Content: fmt.Sprintf("App %s has reached 100%% of its monthly budget (%.2f)",
				app.Name, *app.MonthlyBudget),
			UserID: app.UserID,
		}
		err = db.Create(pastNotification).Error
		require.NoError(t, err)

		// Wait for analytics to process the past records
		waitForAnalytics(t, db, 2)
		waitUntilIdle(t, db)

		// Verify spending for past period
		pastSpent, err := budgetService.GetMonthlySpending(app.ID, budgetStart, budgetStart.AddDate(0, 1, -1))
		require.NoError(t, err)
		assert.InDelta(t, 50.0, pastSpent, 0.1, "Past period spending should be 50.0")

		// Set current time and budget start dates to Feb 15th (new period)
		now = time.Date(2025, 2, 15, 23, 59, 59, 0, loc)
		newPeriodStart := time.Date(2025, 2, 15, 0, 0, 0, 0, loc)
		app.BudgetStartDate = &newPeriodStart
		llm.BudgetStartDate = &newPeriodStart
		err = db.Save(app).Error
		require.NoError(t, err)
		err = db.Save(llm).Error
		require.NoError(t, err)

		// First request in new period should succeed (this will create a record)
		resp, err := http.DefaultClient.Do(makeRequest(false))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Wait for analytics
		waitForAnalytics(t, db, 3)
		waitUntilIdle(t, db)
		record := waitForRecordWithCost(t, db)
		require.NotNil(t, record)

		// Verify spending for current period (Feb 15 - Mar 14)
		periodEnd := time.Date(2025, 3, 14, 23, 59, 59, 0, loc)
		currentSpent, err := budgetService.GetMonthlySpending(app.ID, newPeriodStart, periodEnd)
		require.NoError(t, err)
		assert.InDelta(t, 25.0, currentSpent, 0.1, "Current period spending should be 25.0")
	})
}
