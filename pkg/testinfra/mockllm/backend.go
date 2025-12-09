// Package mockllm provides mock LLM backends for E2E testing.
// These mocks simulate OpenAI-compatible LLM endpoints with configurable
// latency, failure rates, and request tracking.
package mockllm

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// MockLLMBackend represents a mock LLM backend server for testing.
type MockLLMBackend struct {
	Server *httptest.Server
	URL    string

	// Configurable behavior
	latency     time.Duration
	failureRate float32
	model       string
	vendor      string

	// Request tracking
	requestCount atomic.Int64
	requests     []RecordedRequest

	// Concurrency tracking
	activeRequests atomic.Int32

	mu sync.Mutex
}

// RecordedRequest stores details about a received request.
type RecordedRequest struct {
	Timestamp   time.Time
	Path        string
	Method      string
	Body        []byte
	Headers     http.Header
	RequestID   string
	ProcessTime time.Duration
}

// NewMockLLMBackend creates a new mock LLM backend with the given options.
func NewMockLLMBackend(opts ...Option) *MockLLMBackend {
	m := &MockLLMBackend{
		model:   "gpt-4",
		vendor:  "openai",
		latency: 10 * time.Millisecond,
	}

	// Apply options
	for _, opt := range opts {
		opt(m)
	}

	// Create HTTP server
	m.Server = httptest.NewServer(http.HandlerFunc(m.handleRequest))
	m.URL = m.Server.URL

	return m
}

// handleRequest processes incoming requests to the mock backend.
func (m *MockLLMBackend) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	m.activeRequests.Add(1)
	defer m.activeRequests.Add(-1)

	// Read request body
	body := make([]byte, 0)
	if r.Body != nil {
		body, _ = readBody(r)
	}

	// Extract request ID from headers
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}

	// Simulate latency
	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	processTime := time.Since(start)

	// Record the request
	m.mu.Lock()
	m.requestCount.Add(1)
	m.requests = append(m.requests, RecordedRequest{
		Timestamp:   start,
		Path:        r.URL.Path,
		Method:      r.Method,
		Body:        body,
		Headers:     r.Header.Clone(),
		RequestID:   requestID,
		ProcessTime: processTime,
	})
	m.mu.Unlock()

	// Check if we should fail
	if m.failureRate > 0 && rand.Float32() < m.failureRate {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		errorResp := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Internal server error (simulated failure)",
				"type":    "server_error",
				"code":    "internal_error",
			},
		}
		json.NewEncoder(w).Encode(errorResp)
		return
	}

	// Determine response based on path
	switch r.URL.Path {
	case "/v1/chat/completions":
		m.handleChatCompletion(w, r, body)
	case "/health", "/healthz":
		m.handleHealthCheck(w, r)
	default:
		// Default to chat completion for any other path
		m.handleChatCompletion(w, r, body)
	}
}

// handleChatCompletion handles chat completion requests.
func (m *MockLLMBackend) handleChatCompletion(w http.ResponseWriter, r *http.Request, body []byte) {
	// Parse request to get model if specified
	var req map[string]interface{}
	model := m.model
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err == nil {
			if reqModel, ok := req["model"].(string); ok && reqModel != "" {
				model = reqModel
			}
		}
	}

	// Build response
	response := BuildChatCompletionResponse(model)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

// handleHealthCheck handles health check requests.
func (m *MockLLMBackend) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// Close shuts down the mock backend server.
func (m *MockLLMBackend) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// GetRequestCount returns the total number of requests received.
func (m *MockLLMBackend) GetRequestCount() int64 {
	return m.requestCount.Load()
}

// GetActiveRequests returns the number of currently active requests.
func (m *MockLLMBackend) GetActiveRequests() int32 {
	return m.activeRequests.Load()
}

// GetRequests returns all recorded requests.
func (m *MockLLMBackend) GetRequests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]RecordedRequest, len(m.requests))
	copy(result, m.requests)
	return result
}

// Reset clears the request count and recorded requests.
func (m *MockLLMBackend) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCount.Store(0)
	m.requests = nil
}

// SetLatency updates the simulated latency.
func (m *MockLLMBackend) SetLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = d
}

// SetFailureRate updates the failure rate (0.0-1.0).
func (m *MockLLMBackend) SetFailureRate(rate float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// Model returns the model name for this backend.
func (m *MockLLMBackend) Model() string {
	return m.model
}

// Vendor returns the vendor name for this backend.
func (m *MockLLMBackend) Vendor() string {
	return m.vendor
}

// readBody reads the request body without consuming it.
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body := make([]byte, 0, 1024)
	buf := make([]byte, 1024)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return body, nil
}
