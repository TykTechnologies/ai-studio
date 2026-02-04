package proxy

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// TestFilterBlockAnalytics verifies that filter-blocked requests are logged to analytics
func TestFilterBlockAnalytics(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create test request
	reqBody := []byte(`{"prompt": "blocked content"}`)
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), "model_name", "test-model")
	req = req.WithContext(ctx)

	// Simulate a policy violation response body
	respBody := []byte(`{"error":"policy_violation","detail":"Filter: content_filter blocked this request"}`)

	// Call analyzeResponse with a 400 status (what happens when a filter blocks)
	proxy.analyzeResponse(llm, app, http.StatusBadRequest, respBody, reqBody, req)

	// Wait for analytics to process
	time.Sleep(100 * time.Millisecond)

	// Verify ProxyLog was created with correct status code
	var proxyLog models.ProxyLog
	err = db.Where("app_id = ? AND response_code = ?", app.ID, http.StatusBadRequest).First(&proxyLog).Error
	require.NoError(t, err, "ProxyLog should be created for filter-blocked request")

	assert.Equal(t, app.ID, proxyLog.AppID)
	assert.Equal(t, http.StatusBadRequest, proxyLog.ResponseCode)
	assert.Contains(t, proxyLog.ResponseBody, "policy_violation")
}

// TestStreamingFilterBlockAnalytics verifies that streaming filter-blocked requests are logged
func TestStreamingFilterBlockAnalytics(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create test request
	reqBody := []byte(`{"prompt": "blocked streaming content"}`)
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), "model_name", "test-model")
	req = req.WithContext(ctx)

	// Simulate a streaming policy violation response with at least one chunk
	// (analyzeStreamingResponse requires chunks to process)
	respBody := []byte(`{"error":"policy_violation","detail":"Request blocked by filter"}`)
	chunks := [][]byte{respBody}

	// Call analyzeStreamingResponse with a 400 status
	now := time.Now()
	proxy.analyzeStreamingResponse(llm, app, req, http.StatusBadRequest, respBody, reqBody, chunks, now)

	// Wait for analytics to process
	time.Sleep(100 * time.Millisecond)

	// Verify ProxyLog was created
	var proxyLog models.ProxyLog
	err = db.Where("app_id = ? AND response_code = ?", app.ID, http.StatusBadRequest).First(&proxyLog).Error
	require.NoError(t, err, "ProxyLog should be created for streaming filter-blocked request")

	assert.Equal(t, app.ID, proxyLog.AppID)
	assert.Equal(t, http.StatusBadRequest, proxyLog.ResponseCode)
}

// TestInactiveCredentialAnalytics verifies that inactive credential usage is logged
func TestInactiveCredentialAnalytics(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create an INACTIVE credential first
	credential := &models.Credential{
		Model:  gorm.Model{ID: 1},
		KeyID:  "test-key-id",
		Secret: "test-secret-inactive-12345",
		Active: false, // Inactive credential
	}
	err = db.Create(credential).Error
	require.NoError(t, err)

	// Create test app with the credential
	app := &models.App{
		Model:        gorm.Model{ID: 1},
		Name:         "TestApp",
		UserID:       user.ID,
		CredentialID: credential.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create credential validator
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	cv := NewCredentialValidator(service, proxy)

	// Create test request
	req, _ := http.NewRequest("POST", "http://example.com/test", nil)

	// Try to validate the inactive credential
	valid, _ := cv.CheckAPICredential("test-secret-inactive-12345", "", "", "", "", req)
	assert.False(t, valid, "Inactive credential should not be valid")

	// Wait for analytics to process
	time.Sleep(100 * time.Millisecond)

	// Verify ProxyLog was created for the inactive credential attempt
	var proxyLog models.ProxyLog
	err = db.Where("app_id = ? AND response_code = ?", app.ID, http.StatusUnauthorized).First(&proxyLog).Error
	require.NoError(t, err, "ProxyLog should be created for inactive credential usage")

	assert.Equal(t, app.ID, proxyLog.AppID)
	assert.Equal(t, http.StatusUnauthorized, proxyLog.ResponseCode)
	assert.Equal(t, "auth", proxyLog.Vendor)
	assert.Contains(t, proxyLog.ResponseBody, "credential_inactive")
}

// TestAnalyticsStatusCodeCategories verifies different status codes are properly categorized
func TestAnalyticsStatusCodeCategories(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Test various status codes
	testCases := []struct {
		name       string
		statusCode int
		respBody   string
	}{
		{"Success", http.StatusOK, `{"result":"ok"}`},
		{"BadRequest", http.StatusBadRequest, `{"error":"policy_violation"}`},
		{"Unauthorized", http.StatusUnauthorized, `{"error":"invalid_credentials"}`},
		{"Forbidden", http.StatusForbidden, `{"error":"budget_exceeded"}`},
		{"InternalError", http.StatusInternalServerError, `{"error":"upstream_error"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := []byte(`{"prompt": "test"}`)
			req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))

			proxy.analyzeResponse(llm, app, tc.statusCode, []byte(tc.respBody), reqBody, req)

			// Wait for proxy log with proper retry logic for database locks
			waitForProxyLog(t, db, app.ID, tc.statusCode)

			// Verify ProxyLog exists with correct status
			var count int64
			err := db.Model(&models.ProxyLog{}).Where("app_id = ? AND response_code = ?", app.ID, tc.statusCode).Count(&count).Error
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, int64(1), "ProxyLog should be created for status %d", tc.statusCode)
		})
	}
}
