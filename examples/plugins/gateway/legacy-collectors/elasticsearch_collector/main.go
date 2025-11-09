// plugins/examples/elasticsearch_collector/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// ElasticsearchCollector implements DataCollectionPlugin to send data to Elasticsearch
type ElasticsearchCollector struct {
	config *Config
	client *http.Client
}

// Config holds the Elasticsearch plugin configuration
type Config struct {
	ElasticsearchURL  string            `json:"elasticsearch_url"`
	Username          string            `json:"username"`
	Password          string            `json:"password"`
	Indices           map[string]string `json:"indices"`
	BatchSize         int               `json:"batch_size"`
	FlushInterval     string            `json:"flush_interval"`
	UseIndexTemplates bool              `json:"use_index_templates"`
	Timeout           string            `json:"timeout"`
}

// Initialize implements BasePlugin
func (p *ElasticsearchCollector) Initialize(config map[string]interface{}) error {
	// Parse configuration
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	p.config = &Config{}
	if err := json.Unmarshal(configJSON, p.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	
	// Set defaults
	if p.config.ElasticsearchURL == "" {
		p.config.ElasticsearchURL = "http://localhost:9200"
	}
	if p.config.BatchSize == 0 {
		p.config.BatchSize = 100
	}
	if p.config.FlushInterval == "" {
		p.config.FlushInterval = "30s"
	}
	if p.config.Timeout == "" {
		p.config.Timeout = "10s"
	}
	
	// Set default indices if not provided
	if p.config.Indices == nil {
		p.config.Indices = map[string]string{
			"proxy_logs": "microgateway-proxy-logs",
			"analytics":  "microgateway-analytics",
			"budget":     "microgateway-budget",
		}
	}
	
	// Create HTTP client with timeout
	timeout, err := time.ParseDuration(p.config.Timeout)
	if err != nil {
		timeout = 10 * time.Second
	}
	
	p.client = &http.Client{
		Timeout: timeout,
	}
	
	// Test connection to Elasticsearch
	if err := p.testConnection(); err != nil {
		return fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}
	
	return nil
}

// GetHookType implements BasePlugin
func (p *ElasticsearchCollector) GetHookType() sdk.HookType {
	return sdk.HookTypeDataCollection
}

// GetName implements BasePlugin
func (p *ElasticsearchCollector) GetName() string {
	return "elasticsearch-collector"
}

// GetVersion implements BasePlugin
func (p *ElasticsearchCollector) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *ElasticsearchCollector) Shutdown() error {
	// Cleanup resources if needed
	return nil
}

// HandleProxyLog implements DataCollectionPlugin
func (p *ElasticsearchCollector) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// Create Elasticsearch document
	doc := map[string]interface{}{
		"@timestamp":    req.Timestamp.Format(time.RFC3339),
		"app_id":        req.AppID,
		"user_id":       req.UserID,
		"vendor":        req.Vendor,
		"request_body":  string(req.RequestBody),
		"response_body": string(req.ResponseBody),
		"response_code": req.ResponseCode,
		"request_id":    req.RequestID,
		
		// Add metadata from plugin context
		"context": map[string]interface{}{
			"llm_id":   pluginCtx.LLMID,
			"llm_slug": pluginCtx.LLMSlug,
		},
	}
	
	// Index to Elasticsearch
	indexName := p.getIndexName("proxy_logs", req.Timestamp)
	if err := p.indexDocument(ctx, indexName, doc); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"index_name": indexName,
			"timestamp":  req.Timestamp.Format(time.RFC3339),
		},
	}, nil
}

// HandleAnalytics implements DataCollectionPlugin
func (p *ElasticsearchCollector) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// Create Elasticsearch document
	doc := map[string]interface{}{
		"@timestamp":                   req.Timestamp.Format(time.RFC3339),
		"llm_id":                      req.LLMID,
		"model_name":                  req.ModelName,
		"vendor":                      req.Vendor,
		"prompt_tokens":               req.PromptTokens,
		"response_tokens":             req.ResponseTokens,
		"cache_write_prompt_tokens":   req.CacheWritePromptTokens,
		"cache_read_prompt_tokens":    req.CacheReadPromptTokens,
		"total_tokens":                req.TotalTokens,
		"cost":                        req.Cost,
		"currency":                    req.Currency,
		"app_id":                      req.AppID,
		"user_id":                     req.UserID,
		"tool_calls":                  req.ToolCalls,
		"choices":                     req.Choices,
		"request_id":                  req.RequestID,
		
		// Add metadata from plugin context
		"context": map[string]interface{}{
			"llm_slug": pluginCtx.LLMSlug,
		},
	}
	
	// Index to Elasticsearch
	indexName := p.getIndexName("analytics", req.Timestamp)
	if err := p.indexDocument(ctx, indexName, doc); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"index_name": indexName,
			"timestamp":  req.Timestamp.Format(time.RFC3339),
		},
	}, nil
}

// HandleBudgetUsage implements DataCollectionPlugin
func (p *ElasticsearchCollector) HandleBudgetUsage(ctx context.Context, req *sdk.BudgetUsageData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// Create Elasticsearch document
	doc := map[string]interface{}{
		"@timestamp":         req.Timestamp.Format(time.RFC3339),
		"app_id":            req.AppID,
		"llm_id":            req.LLMID,
		"tokens_used":       req.TokensUsed,
		"cost":              req.Cost,
		"requests_count":    req.RequestsCount,
		"prompt_tokens":     req.PromptTokens,
		"completion_tokens": req.CompletionTokens,
		"period_start":      req.PeriodStart.Format(time.RFC3339),
		"period_end":        req.PeriodEnd.Format(time.RFC3339),
		"request_id":        req.RequestID,
	}
	
	// Index to Elasticsearch
	indexName := p.getIndexName("budget", req.Timestamp)
	if err := p.indexDocument(ctx, indexName, doc); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"index_name": indexName,
			"timestamp":  req.Timestamp.Format(time.RFC3339),
		},
	}, nil
}

// testConnection tests the connection to Elasticsearch
func (p *ElasticsearchCollector) testConnection() error {
	req, err := http.NewRequest("GET", p.config.ElasticsearchURL+"/_cluster/health", nil)
	if err != nil {
		return err
	}
	
	if p.config.Username != "" && p.config.Password != "" {
		req.SetBasicAuth(p.config.Username, p.config.Password)
	}
	
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Elasticsearch health check failed with status %d", resp.StatusCode)
	}
	
	return nil
}

// getIndexName generates index name with optional date suffix
func (p *ElasticsearchCollector) getIndexName(dataType string, timestamp time.Time) string {
	baseIndex := p.config.Indices[dataType]
	if baseIndex == "" {
		baseIndex = "microgateway-" + dataType
	}
	
	// Add date suffix for time-based indices
	return baseIndex + "-" + timestamp.Format("2006.01.02")
}

// indexDocument indexes a document to Elasticsearch
func (p *ElasticsearchCollector) indexDocument(ctx context.Context, indexName string, doc map[string]interface{}) error {
	// Convert document to JSON
	jsonDoc, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}
	
	// Create index URL with auto-generated ID
	url := fmt.Sprintf("%s/%s/_doc", p.config.ElasticsearchURL, indexName)
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonDoc))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	
	// Add authentication if configured
	if p.config.Username != "" && p.config.Password != "" {
		req.SetBasicAuth(p.config.Username, p.config.Password)
	}
	
	// Execute request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Elasticsearch indexing failed with status %d", resp.StatusCode)
	}
	
	return nil
}

func main() {
	plugin := &ElasticsearchCollector{}
	sdk.ServePlugin(plugin)
}