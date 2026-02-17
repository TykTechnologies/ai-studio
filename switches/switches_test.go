package switches

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms"
)

func TestDetectStreamingIntent_OpenAI(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		expected    bool
		expectError bool
	}{
		{
			name:     "OpenAI streaming request",
			body:     `{"model": "gpt-4", "messages": [], "stream": true}`,
			expected: true,
		},
		{
			name:     "OpenAI non-streaming request",
			body:     `{"model": "gpt-4", "messages": [], "stream": false}`,
			expected: false,
		},
		{
			name:     "OpenAI request without stream field",
			body:     `{"model": "gpt-4", "messages": []}`,
			expected: false,
		},
		{
			name:        "Invalid JSON",
			body:        `invalid json`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Body: io.NopCloser(bytes.NewReader([]byte(tt.body))),
			}

			isStreaming, err := DetectStreamingIntent(models.OPENAI, req)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if isStreaming != tt.expected {
				t.Errorf("Expected streaming=%v but got %v", tt.expected, isStreaming)
			}

			// Verify body can still be read after detection
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("Failed to read body after detection: %v", err)
			}
			if string(bodyBytes) != tt.body {
				t.Errorf("Body was not properly restored. Expected: %s, Got: %s", tt.body, string(bodyBytes))
			}
		})
	}
}

func TestDetectStreamingIntent_Anthropic(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "Anthropic streaming request",
			body:     `{"model": "claude-3-5-sonnet-20241022", "messages": [], "stream": true, "max_tokens": 1024}`,
			expected: true,
		},
		{
			name:     "Anthropic non-streaming request",
			body:     `{"model": "claude-3-5-sonnet-20241022", "messages": [], "stream": false, "max_tokens": 1024}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Body: io.NopCloser(bytes.NewReader([]byte(tt.body))),
			}

			isStreaming, err := DetectStreamingIntent(models.ANTHROPIC, req)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if isStreaming != tt.expected {
				t.Errorf("Expected streaming=%v but got %v", tt.expected, isStreaming)
			}
		})
	}
}

func TestDetectStreamingIntent_GoogleAI(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		query    string
		expected bool
	}{
		{
			name:     "Google AI with streamGenerateContent in path",
			path:     "/v1beta/models/gemini-pro:streamGenerateContent",
			expected: true,
		},
		{
			name:     "Google AI with generateContent in path",
			path:     "/v1beta/models/gemini-pro:generateContent",
			expected: false,
		},
		{
			name:     "Google AI with alt=sse query param",
			path:     "/v1beta/models/gemini-pro:generateContent",
			query:    "alt=sse",
			expected: true,
		},
		{
			name:     "Google AI case insensitive path check",
			path:     "/v1beta/models/gemini-pro:STREAMGENERATECONTENT",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://example.com" + tt.path)
			if tt.query != "" {
				u.RawQuery = tt.query
			}

			req := &http.Request{
				URL:  u,
				Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
			}

			isStreaming, err := DetectStreamingIntent(models.GOOGLEAI, req)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if isStreaming != tt.expected {
				t.Errorf("Expected streaming=%v but got %v for path=%s query=%s", tt.expected, isStreaming, tt.path, tt.query)
			}
		})
	}
}

func TestDetectStreamingIntent_Vertex(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Vertex with streamGenerateContent",
			path:     "/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-pro:streamGenerateContent",
			expected: true,
		},
		{
			name:     "Vertex with generateContent",
			path:     "/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-pro:generateContent",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://example.com" + tt.path)
			req := &http.Request{
				URL:  u,
				Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
			}

			isStreaming, err := DetectStreamingIntent(models.VERTEX, req)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if isStreaming != tt.expected {
				t.Errorf("Expected streaming=%v but got %v", tt.expected, isStreaming)
			}
		})
	}
}

func TestDetectStreamingIntent_Ollama(t *testing.T) {
	streamingBody := `{"model": "llama2", "prompt": "Hello", "stream": true}`
	nonStreamingBody := `{"model": "llama2", "prompt": "Hello", "stream": false}`

	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "Ollama streaming request",
			body:     streamingBody,
			expected: true,
		},
		{
			name:     "Ollama non-streaming request",
			body:     nonStreamingBody,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Body: io.NopCloser(bytes.NewReader([]byte(tt.body))),
			}

			isStreaming, err := DetectStreamingIntent(models.OLLAMA, req)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if isStreaming != tt.expected {
				t.Errorf("Expected streaming=%v but got %v", tt.expected, isStreaming)
			}
		})
	}
}

func TestDetectStreamingIntent_UnsupportedVendor(t *testing.T) {
	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
	}

	_, err := DetectStreamingIntent("unsupported_vendor", req)

	if err == nil {
		t.Error("Expected error for unsupported vendor but got none")
	}
}

// mockRoundTripper is a custom HTTP transport that captures requests
type mockRoundTripper struct {
	capturedRequests []*http.Request
	response         *http.Response
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedRequests = append(m.capturedRequests, req)
	if m.response != nil {
		return m.response, nil
	}
	// Return a default response
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}, nil
}

func TestFetchDriver_WithHTTPClient_GoogleAI(t *testing.T) {
	tests := []struct {
		name            string
		llmConfig       *models.LLM
		settings        *models.LLMSettings
		wantErr         bool
		expectedBaseURL string
	}{
		{
			name: "GoogleAI with custom HTTP client and API endpoint",
			llmConfig: &models.LLM{
				Vendor:      models.GOOGLEAI,
				APIKey:      "test-api-key",
				APIEndpoint: "https://custom-endpoint.example.com",
			},
			settings: &models.LLMSettings{
				ModelName: "gemini-pro",
			},
			wantErr:         false,
			expectedBaseURL: "https://custom-endpoint.example.com",
		},
		{
			name: "GoogleAI with custom HTTP client without API endpoint(the baseURL will be added by SDK with value: https://generativelanguage.googleapis.com)",
			llmConfig: &models.LLM{
				Vendor: models.GOOGLEAI,
				APIKey: "test-api-key",
			},
			settings: &models.LLMSettings{
				ModelName: "gemini-pro",
			},
			wantErr:         false,
			expectedBaseURL: "https://generativelanguage.googleapis.com",
		},
		{
			name: "GoogleAI with custom HTTP client and empty settings",
			llmConfig: &models.LLM{
				Vendor:      models.GOOGLEAI,
				APIKey:      "test-api-key",
				APIEndpoint: "https://another-endpoint.example.com",
			},
			settings:        nil,
			wantErr:         false,
			expectedBaseURL: "https://another-endpoint.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTransport := &mockRoundTripper{}
			httpClient := &http.Client{
				Transport: mockTransport,
			}

			driver, err := FetchDriver(
				tt.llmConfig,
				tt.settings,
				nil,
				nil,
				WithHTTPClient(httpClient),
			)
			if tt.wantErr {
				assert.NotNil(t, err, "FetchDriver() succeeded unexpectedly")
				return
			}
			assert.NotNil(t, driver, "FetchDriver() returned nil driver")

			_, err = driver.GenerateContent(context.Background(), []llms.MessageContent{
				{
					Role: llms.ChatMessageTypeHuman,
					Parts: []llms.ContentPart{
						llms.TextContent{Text: "test message"},
					},
				},
			})
			assert.Error(t, err, "GenerateContent() succeeded unexpectedly with mocked transport")
			assert.Len(t, mockTransport.capturedRequests, 1)

			capturedReq := mockTransport.capturedRequests[0]
			capturedReqURL := capturedReq.URL.Scheme + "://" + capturedReq.URL.Host
			assert.Equal(t, tt.expectedBaseURL, capturedReqURL, fmt.Sprintf(
				"Expected base URL %s, but got %s",
				tt.expectedBaseURL,
				capturedReqURL,
			))
		})
	}
}

func TestFetchDriver_WithHTTPClient_AllVendors(t *testing.T) {
	tests := []struct {
		name      string
		vendor    models.Vendor
		llmConfig *models.LLM
		settings  *models.LLMSettings
		wantErr   bool
	}{
		{
			name:   "OpenAI with custom HTTP client",
			vendor: models.OPENAI,
			llmConfig: &models.LLM{
				Vendor:      models.OPENAI,
				APIKey:      "test-key",
				APIEndpoint: "https://api.openai.com/v1",
			},
			settings: &models.LLMSettings{
				ModelName: "gpt-4",
			},
		},
		{
			name:   "Anthropic with custom HTTP client",
			vendor: models.ANTHROPIC,
			llmConfig: &models.LLM{
				Vendor:      models.ANTHROPIC,
				APIKey:      "test-key",
				APIEndpoint: "https://api.anthropic.com",
			},
			settings: &models.LLMSettings{
				ModelName: "claude-3-5-sonnet-20241022",
			},
		},
		{
			name:   "Ollama with custom HTTP client",
			vendor: models.OLLAMA,
			llmConfig: &models.LLM{
				Vendor:      models.OLLAMA,
				APIEndpoint: "http://localhost:11434",
			},
			settings: &models.LLMSettings{
				ModelName: "llama2",
			},
		},
		{
			name:   "Vertex with custom HTTP client",
			vendor: models.VERTEX,
			llmConfig: &models.LLM{
				Vendor: models.VERTEX,
				APIKey: "test-key",
			},
			settings: &models.LLMSettings{
				ModelName: "gemini-pro",
			},
		},
		{
			name:   "Unsupported LLM",
			vendor: models.MOCK_VENDOR,
			llmConfig: &models.LLM{
				Vendor: models.MOCK_VENDOR,
				APIKey: "test-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTransport := &mockRoundTripper{}
			httpClient := &http.Client{
				Transport: mockTransport,
			}

			driver, err := FetchDriver(
				tt.llmConfig,
				tt.settings,
				nil,
				nil,
				WithHTTPClient(httpClient),
			)
			if tt.wantErr {
				assert.Error(t, err, "FetchDriver() succeeded unexpectedly")
				return
			}

			assert.NotNil(t, driver, "FetchDriver() returned nil driver")
		})
	}
}
