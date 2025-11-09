// plugins/examples/file_analytics_collector/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// FileAnalyticsCollector implements DataCollectionPlugin to append analytics data to text files
type FileAnalyticsCollector struct {
	outputDir string
	enabled   bool
	csvMode   bool
}

// Initialize implements BasePlugin
func (p *FileAnalyticsCollector) Initialize(config map[string]interface{}) error {
	// Parse configuration
	outputDir, ok := config["output_directory"].(string)
	if !ok || outputDir == "" {
		outputDir = "./data/analytics"
	}
	p.outputDir = outputDir

	// Check if enabled
	if enabled, ok := config["enabled"].(bool); ok {
		p.enabled = enabled
	} else {
		p.enabled = true
	}

	// Check output format
	if format, ok := config["format"].(string); ok && format == "csv" {
		p.csvMode = true
	} else {
		p.csvMode = false
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", p.outputDir, err)
	}

	// Create CSV header if in CSV mode
	if p.csvMode {
		if err := p.ensureCSVHeader(); err != nil {
			return fmt.Errorf("failed to create CSV header: %w", err)
		}
	}

	return nil
}

// GetHookType implements BasePlugin
func (p *FileAnalyticsCollector) GetHookType() sdk.HookType {
	return sdk.HookTypeDataCollection
}

// GetName implements BasePlugin
func (p *FileAnalyticsCollector) GetName() string {
	return "file-analytics-collector"
}

// GetVersion implements BasePlugin
func (p *FileAnalyticsCollector) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *FileAnalyticsCollector) Shutdown() error {
	return nil
}

// HandleProxyLog implements DataCollectionPlugin - not used by this plugin
func (p *FileAnalyticsCollector) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles analytics
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleAnalytics implements DataCollectionPlugin - this is the main functionality
func (p *FileAnalyticsCollector) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	if !p.enabled {
		return &sdk.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	var filename, content string
	var err error

	if p.csvMode {
		filename = fmt.Sprintf("analytics_%s.csv", req.Timestamp.Format("2006-01-02"))
		content = fmt.Sprintf("%s,%d,%s,%s,%d,%d,%d,%d,%d,%.6f,%s,%d,%d,%d,%d,%s\n",
			req.Timestamp.Format(time.RFC3339),
			req.LLMID,
			req.ModelName,
			req.Vendor,
			req.PromptTokens,
			req.ResponseTokens,
			req.CacheWritePromptTokens,
			req.CacheReadPromptTokens,
			req.TotalTokens,
			req.Cost,
			req.Currency,
			req.AppID,
			req.UserID,
			req.ToolCalls,
			req.Choices,
			req.RequestID,
		)
	} else {
		// JSON Lines format
		filename = fmt.Sprintf("analytics_%s.jsonl", req.Timestamp.Format("2006-01-02"))
		
		logEntry := map[string]interface{}{
			"timestamp":                    req.Timestamp.Format(time.RFC3339),
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
			"context": map[string]interface{}{
				"llm_slug": pluginCtx.LLMSlug,
			},
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			return &sdk.DataCollectionResponse{
				Success:      false,
				Handled:      false,
				ErrorMessage: fmt.Sprintf("failed to marshal analytics entry: %v", err),
			}, nil
		}
		content = string(jsonData) + "\n"
	}

	// Write to file
	filepath := filepath.Join(p.outputDir, filename)
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to open analytics file %s: %v", filepath, err),
		}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write analytics data: %v", err),
		}, nil
	}

	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"file_path": filepath,
			"format":    getOutputFormat(p.csvMode),
		},
	}, nil
}

// HandleBudgetUsage implements DataCollectionPlugin - not used by this plugin
func (p *FileAnalyticsCollector) HandleBudgetUsage(ctx context.Context, req *sdk.BudgetUsageData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles analytics
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// ensureCSVHeader creates CSV header if file doesn't exist
func (p *FileAnalyticsCollector) ensureCSVHeader() error {
	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("analytics_%s.csv", today)
	filepath := filepath.Join(p.outputDir, filename)

	// Check if file exists
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		// Create file with header
		file, err := os.Create(filepath)
		if err != nil {
			return err
		}
		defer file.Close()

		header := "timestamp,llm_id,model_name,vendor,prompt_tokens,response_tokens,cache_write_tokens,cache_read_tokens,total_tokens,cost,currency,app_id,user_id,tool_calls,choices,request_id\n"
		_, err = file.WriteString(header)
		return err
	}

	return nil
}

func getOutputFormat(csvMode bool) string {
	if csvMode {
		return "csv"
	}
	return "jsonl"
}

func main() {
	plugin := &FileAnalyticsCollector{}
	sdk.ServePlugin(plugin)
}