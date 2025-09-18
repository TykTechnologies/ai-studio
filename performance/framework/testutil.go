// Package framework provides utilities for performance testing across the Midsommar system
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkDB provides a test database with query monitoring for benchmarks
type BenchmarkDB struct {
	DB           *gorm.DB
	QueryLogger  *QueryCountLogger
	setupData    sync.Once
	testDataSize int
}

// QueryCountLogger captures database queries for performance analysis
type QueryCountLogger struct {
	QueryCount int
	Queries    []string
	mu         sync.Mutex
}

func (l *QueryCountLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *QueryCountLogger) Info(ctx context.Context, msg string, data ...interface{})  {}
func (l *QueryCountLogger) Warn(ctx context.Context, msg string, data ...interface{})  {}
func (l *QueryCountLogger) Error(ctx context.Context, msg string, data ...interface{}) {}

func (l *QueryCountLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	l.QueryCount++
	l.Queries = append(l.Queries, sql)
	l.mu.Unlock()
}

// Reset clears the query counter and logs
func (l *QueryCountLogger) Reset() {
	l.mu.Lock()
	l.QueryCount = 0
	l.Queries = []string{}
	l.mu.Unlock()
}

// GetStats returns current query statistics
func (l *QueryCountLogger) GetStats() (count int, queries []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Return copies to avoid race conditions
	return l.QueryCount, append([]string{}, l.Queries...)
}

// NewBenchmarkDB creates a new test database with performance monitoring
func NewBenchmarkDB(t *testing.B) *BenchmarkDB {
	queryLogger := &QueryCountLogger{}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: queryLogger,
		// Optimize for performance testing
		SkipDefaultTransaction: true,
		PrepareStmt:           true,
	})
	require.NoError(t, err)

	// Migrate all required tables
	err = db.AutoMigrate(
		&models.User{},
		&models.App{},
		&models.Credential{},
		&models.LLM{},
		&models.Plugin{},
		&models.Filter{},
		&models.LLMPlugin{},
		&models.LLMChatRecord{},
		&models.ProxyLog{},
		&models.EdgeInstance{},
		&models.ModelPrice{},
		&models.LLMSettings{},
	)
	require.NoError(t, err)

	return &BenchmarkDB{
		DB:           db,
		QueryLogger:  queryLogger,
		testDataSize: 10, // Default test data size
	}
}

// SetTestDataSize configures the amount of test data to create
func (bdb *BenchmarkDB) SetTestDataSize(size int) *BenchmarkDB {
	bdb.testDataSize = size
	return bdb
}

// SetupTestData creates test data for performance benchmarks (idempotent)
func (bdb *BenchmarkDB) SetupTestData(t *testing.B) {
	bdb.setupData.Do(func() {
		bdb.createTestData(t)
	})
}

// createTestData populates the database with test data
func (bdb *BenchmarkDB) createTestData(t *testing.B) {
	// Create test users
	users := make([]models.User, bdb.testDataSize)
	for i := 0; i < bdb.testDataSize; i++ {
		users[i] = models.User{
			Model: gorm.Model{ID: uint(i + 1)},
			Name:  fmt.Sprintf("TestUser%d", i+1),
			Email: fmt.Sprintf("user%d@test.com", i+1),
		}
	}
	err := bdb.DB.Create(&users).Error
	require.NoError(t, err)

	// Create test credentials first (they need to exist before apps reference them)
	credentials := make([]models.Credential, bdb.testDataSize)
	for i := 0; i < bdb.testDataSize; i++ {
		credentials[i] = models.Credential{
			Model:  gorm.Model{ID: uint(i + 1)},
			KeyID:  fmt.Sprintf("test-key-%d", i+1),
			Secret: fmt.Sprintf("secret-hash-%d", i+1),
			Active: true,
		}
	}
	err = bdb.DB.Create(&credentials).Error
	require.NoError(t, err)

	// Create test apps with proper credential associations
	apps := make([]models.App, bdb.testDataSize)
	for i := 0; i < bdb.testDataSize; i++ {
		apps[i] = models.App{
			Model:         gorm.Model{ID: uint(i + 1)},
			Name:          fmt.Sprintf("TestApp%d", i+1),
			UserID:        uint(i%len(users) + 1), // Distribute across users
			CredentialID:  uint(i + 1),           // Link to the credential
			IsActive:      true,
			MonthlyBudget: &[]float64{1000.0}[0],
		}
	}
	err = bdb.DB.Create(&apps).Error
	require.NoError(t, err)

	// Create test LLMs with realistic configuration (using names that match benchmark URLs)
	llms := make([]models.LLM, bdb.testDataSize)
	vendors := []models.Vendor{models.OPENAI, models.ANTHROPIC, models.GOOGLEAI, models.VERTEX}
	for i := 0; i < bdb.testDataSize; i++ {
		name := fmt.Sprintf("test-llm-%d", i+1)
		llms[i] = models.LLM{
			Model:        gorm.Model{ID: uint(i + 1)},
			Name:         name,                     // Match benchmark URL pattern
			Vendor:       vendors[i%len(vendors)],
			DefaultModel: "gpt-4",                 // Use a standard model name
			Active:       true,
			Namespace:    "",
			APIKey:       "test-api-key-" + fmt.Sprintf("%d", i+1), // Add API key for completeness
		}
	}
	err = bdb.DB.Create(&llms).Error
	require.NoError(t, err)

	// Create test plugins
	pluginCount := bdb.testDataSize * 2 // More plugins than LLMs
	plugins := make([]models.Plugin, pluginCount)
	hookTypes := []string{"pre_auth", "auth", "post_auth", "pre_request", "post_request"}
	for i := 0; i < pluginCount; i++ {
		plugins[i] = models.Plugin{
			Model:       gorm.Model{ID: uint(i + 1)},
			Name:        fmt.Sprintf("TestPlugin%d", i+1),
			Slug:        fmt.Sprintf("test-plugin-%d", i+1),
			Description: fmt.Sprintf("Test plugin %d for benchmarking", i+1),
			Command:     fmt.Sprintf("./plugin%d", i+1),
			HookType:    hookTypes[i%len(hookTypes)],
			IsActive:    true,
			Namespace:   "",
		}
	}
	err = bdb.DB.Create(&plugins).Error
	require.NoError(t, err)

	// Create LLM-Plugin associations (realistic many-to-many relationships)
	var associations []models.LLMPlugin
	for llmID := 1; llmID <= bdb.testDataSize; llmID++ {
		pluginCount := 3 + (llmID % 4) // 3-6 plugins per LLM
		for pluginID := 1; pluginID <= pluginCount && pluginID <= len(plugins); pluginID++ {
			associations = append(associations, models.LLMPlugin{
				LLMID:    uint(llmID),
				PluginID: uint(pluginID),
				IsActive: true,
			})
		}
	}
	err = bdb.DB.Create(&associations).Error
	require.NoError(t, err)

	// Create App-LLM associations (apps can access LLMs for testing)
	for appID := 1; appID <= bdb.testDataSize; appID++ {
		// Each app gets access to a few LLMs for testing
		for llmID := 1; llmID <= min(3, bdb.testDataSize); llmID++ {
			// Use the actual app and LLM instances created above
			app := &apps[appID-1]
			llm := &llms[llmID-1]
			err = bdb.DB.Model(app).Association("LLMs").Append(llm)
			require.NoError(t, err)
		}
	}

	// Create model pricing data
	modelPrices := []models.ModelPrice{
		{Model: gorm.Model{ID: 1}, ModelName: "gpt-4", Vendor: "openai", CPIT: 30, CPT: 60, Currency: "USD"},
		{Model: gorm.Model{ID: 2}, ModelName: "gpt-3.5-turbo", Vendor: "openai", CPIT: 1, CPT: 2, Currency: "USD"},
		{Model: gorm.Model{ID: 3}, ModelName: "claude-3-haiku", Vendor: "anthropic", CPIT: 2, CPT: 10, Currency: "USD"},
		{Model: gorm.Model{ID: 4}, ModelName: "claude-3-sonnet", Vendor: "anthropic", CPIT: 3, CPT: 15, Currency: "USD"},
	}
	err = bdb.DB.Create(&modelPrices).Error
	require.NoError(t, err)
}

// ResetQueryStats resets the query counter for a new benchmark run
func (bdb *BenchmarkDB) ResetQueryStats() {
	bdb.QueryLogger.Reset()
}

// GetDB returns the database instance for testing
func (bdb *BenchmarkDB) GetDB() *gorm.DB {
	return bdb.DB
}

// MockLLMServer provides a mock LLM provider for testing
type MockLLMServer struct {
	server   *httptest.Server
	Requests []MockRequest
	mu       sync.Mutex
}

// MockRequest captures request details for analysis
type MockRequest struct {
	Method      string
	Path        string
	Headers     http.Header
	Body        []byte
	Timestamp   time.Time
	ResponseTime time.Duration
}

// NewMockLLMServer creates a mock LLM provider server
func NewMockLLMServer() *MockLLMServer {
	mock := &MockLLMServer{
		Requests: make([]MockRequest, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))
	return mock
}

// handleRequest simulates an LLM provider response
func (m *MockLLMServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Capture request details
	body := make([]byte, 0)
	if r.Body != nil {
		bodyBytes := make([]byte, r.ContentLength)
		r.Body.Read(bodyBytes)
		body = bodyBytes
	}

	req := MockRequest{
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   r.Header.Clone(),
		Body:      body,
		Timestamp: start,
	}

	// Simulate realistic response times based on path
	var delay time.Duration
	switch {
	case r.URL.Path == "/v1/chat/completions" && !isStreaming(r):
		delay = time.Millisecond * time.Duration(50+rand.Intn(100)) // 50-150ms for REST
	case r.URL.Path == "/v1/chat/completions" && isStreaming(r):
		delay = time.Millisecond * time.Duration(200+rand.Intn(300)) // 200-500ms for streaming
	default:
		delay = time.Millisecond * time.Duration(10+rand.Intn(40)) // 10-50ms for other endpoints
	}

	time.Sleep(delay)

	// Generate appropriate response
	if isStreaming(r) {
		m.writeStreamingResponse(w)
	} else {
		m.writeRESTResponse(w)
	}

	req.ResponseTime = time.Since(start)

	// Store request details
	m.mu.Lock()
	m.Requests = append(m.Requests, req)
	m.mu.Unlock()
}

// isStreaming checks if the request expects streaming response
func isStreaming(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "text/event-stream" || r.Header.Get("Stream") == "true"
}

// writeRESTResponse writes a standard JSON response
func (m *MockLLMServer) writeRESTResponse(w http.ResponseWriter) {
	response := map[string]interface{}{
		"id":      fmt.Sprintf("cmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "This is a mock response for performance testing.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     50,
			"completion_tokens": 20,
			"total_tokens":      70,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// writeStreamingResponse writes a streaming response
func (m *MockLLMServer) writeStreamingResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Simulate streaming chunks
	chunks := []string{
		"This", " is", " a", " mock", " streaming", " response", " for", " performance", " testing", ".",
	}

	for _, chunk := range chunks {
		data := map[string]interface{}{
			"id":      fmt.Sprintf("cmpl-%d", time.Now().UnixNano()),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]string{
						"content": chunk,
					},
				},
			},
		}

		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()

		// Small delay between chunks to simulate realistic streaming
		time.Sleep(time.Millisecond * 10)
	}

	// Send final chunk
	finalData := map[string]interface{}{
		"id":      fmt.Sprintf("cmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []map[string]interface{}{
			{
				"index":        0,
				"delta":        map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
	}
	jsonData, _ := json.Marshal(finalData)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// URL returns the mock server URL
func (m *MockLLMServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockLLMServer) Close() {
	m.server.Close()
}

// GetRequestStats returns statistics about received requests
func (m *MockLLMServer) GetRequestStats() RequestStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Requests) == 0 {
		return RequestStats{}
	}

	stats := RequestStats{
		TotalRequests: len(m.Requests),
	}

	var totalResponseTime time.Duration
	for _, req := range m.Requests {
		totalResponseTime += req.ResponseTime
		if req.ResponseTime > stats.MaxResponseTime {
			stats.MaxResponseTime = req.ResponseTime
		}
		if stats.MinResponseTime == 0 || req.ResponseTime < stats.MinResponseTime {
			stats.MinResponseTime = req.ResponseTime
		}
	}

	stats.AvgResponseTime = totalResponseTime / time.Duration(len(m.Requests))

	return stats
}

// RequestStats contains statistics about mock server requests
type RequestStats struct {
	TotalRequests   int
	AvgResponseTime time.Duration
	MinResponseTime time.Duration
	MaxResponseTime time.Duration
}

// BuildLLMRequest creates a test LLM request
func BuildLLMRequest(model string, messages []map[string]string) []byte {
	request := map[string]interface{}{
		"model":     model,
		"messages":  messages,
		"stream":    false,
		"max_tokens": 100,
	}

	data, _ := json.Marshal(request)
	return data
}

// BuildStreamingLLMRequest creates a test streaming LLM request
func BuildStreamingLLMRequest(model string, messages []map[string]string) []byte {
	request := map[string]interface{}{
		"model":     model,
		"messages":  messages,
		"stream":    true,
		"max_tokens": 100,
	}

	data, _ := json.Marshal(request)
	return data
}

// Simple random number generator for deterministic testing
var rand = &simpleRand{seed: time.Now().UnixNano()}

type simpleRand struct {
	seed int64
}

func (r *simpleRand) Intn(n int) int {
	r.seed = r.seed*1103515245 + 12345
	return int((r.seed / 65536) % int64(n))
}