// plugins/examples/file_proxy_collector/main.go
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

// FileProxyCollector implements DataCollectionPlugin to append proxy logs to text files
type FileProxyCollector struct {
	outputDir string
	enabled   bool
}

// Initialize implements BasePlugin
func (p *FileProxyCollector) Initialize(config map[string]interface{}) error {
	// Parse configuration
	outputDir, ok := config["output_directory"].(string)
	if !ok || outputDir == "" {
		outputDir = "./data/proxy_logs"
	}
	p.outputDir = outputDir

	// Check if enabled
	if enabled, ok := config["enabled"].(bool); ok {
		p.enabled = enabled
	} else {
		p.enabled = true
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", p.outputDir, err)
	}

	return nil
}

// GetHookType implements BasePlugin
func (p *FileProxyCollector) GetHookType() sdk.HookType {
	return sdk.HookTypeDataCollection
}

// GetName implements BasePlugin
func (p *FileProxyCollector) GetName() string {
	return "file-proxy-collector"
}

// GetVersion implements BasePlugin
func (p *FileProxyCollector) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *FileProxyCollector) Shutdown() error {
	return nil
}

// HandleProxyLog implements DataCollectionPlugin - this is the main functionality
func (p *FileProxyCollector) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	if !p.enabled {
		return &sdk.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	// Create log entry
	logEntry := map[string]interface{}{
		"timestamp":     req.Timestamp.Format(time.RFC3339),
		"app_id":        req.AppID,
		"user_id":       req.UserID,
		"vendor":        req.Vendor,
		"response_code": req.ResponseCode,
		"request_id":    req.RequestID,
		"request_size":  len(req.RequestBody),
		"response_size": len(req.ResponseBody),
		// Include first 200 chars of request/response for debugging
		"request_preview":  truncateString(string(req.RequestBody), 200),
		"response_preview": truncateString(string(req.ResponseBody), 200),
		"context": map[string]interface{}{
			"llm_id":   pluginCtx.LLMID,
			"llm_slug": pluginCtx.LLMSlug,
		},
	}

	// Convert to JSON
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to marshal log entry: %v", err),
		}, nil
	}

	// Append to daily log file
	filename := fmt.Sprintf("proxy_logs_%s.jsonl", req.Timestamp.Format("2006-01-02"))
	filepath := filepath.Join(p.outputDir, filename)

	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to open log file %s: %v", filepath, err),
		}, nil
	}
	defer file.Close()

	// Write JSON line
	if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write to log file: %v", err),
		}, nil
	}

	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"file_path": filepath,
			"file_size": getFileSize(filepath),
		},
	}, nil
}

// HandleAnalytics implements DataCollectionPlugin - not used by this plugin
func (p *FileProxyCollector) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles proxy logs
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleBudgetUsage implements DataCollectionPlugin - not used by this plugin
func (p *FileProxyCollector) HandleBudgetUsage(ctx context.Context, req *sdk.BudgetUsageData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles proxy logs
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// Helper functions

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getFileSize(filepath string) int64 {
	if info, err := os.Stat(filepath); err == nil {
		return info.Size()
	}
	return 0
}

func main() {
	plugin := &FileProxyCollector{}
	sdk.ServePlugin(plugin)
}