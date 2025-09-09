// internal/cli/client.go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Client represents the HTTP client for microgateway API
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// APIResponse represents a standard API response
type APIResponse struct {
	Data       interface{} `json:"data,omitempty"`
	Message    string      `json:"message,omitempty"`
	Error      string      `json:"error,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination represents pagination information
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// NewClient creates a new microgateway API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// makeRequest makes an HTTP request to the microgateway API
func (c *Client) makeRequest(method, endpoint string, body interface{}) (*APIResponse, error) {
	// Prepare request URL
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = path.Join(u.Path, endpoint)

	// Prepare request body
	var requestBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	// Create HTTP request
	req, err := http.NewRequest(method, u.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		if apiResp.Error != "" {
			// Include detailed message if available
			if apiResp.Message != "" {
				return nil, fmt.Errorf("API error (%d): %s - %s", resp.StatusCode, apiResp.Error, apiResp.Message)
			}
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error)
		}
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	return &apiResp, nil
}

// GET request
func (c *Client) Get(endpoint string) (*APIResponse, error) {
	return c.makeRequest("GET", endpoint, nil)
}

// POST request
func (c *Client) Post(endpoint string, body interface{}) (*APIResponse, error) {
	return c.makeRequest("POST", endpoint, body)
}

// PUT request
func (c *Client) Put(endpoint string, body interface{}) (*APIResponse, error) {
	return c.makeRequest("PUT", endpoint, body)
}

// DELETE request
func (c *Client) Delete(endpoint string) (*APIResponse, error) {
	return c.makeRequest("DELETE", endpoint, nil)
}

// GetWithQuery makes a GET request with query parameters
func (c *Client) GetWithQuery(endpoint string, params map[string]string) (*APIResponse, error) {
	if len(params) == 0 {
		return c.Get(endpoint)
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = path.Join(u.Path, endpoint)

	// Add query parameters
	q := u.Query()
	for key, value := range params {
		q.Add(key, value)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if apiResp.Error != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiResp.Error)
		}
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	return &apiResp, nil
}