// plugins/examples/unified/data-collectors/file-proxy-collector/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// FileProxyCollector implements DataCollector to append proxy logs to text files
type FileProxyCollector struct {
	plugin_sdk.BasePlugin
	outputDir string
	enabled   bool
}

// NewFileProxyCollector creates a new instance of the plugin
func NewFileProxyCollector() *FileProxyCollector {
	return &FileProxyCollector{
		BasePlugin: plugin_sdk.NewBasePlugin(
			"file-proxy-collector",
			"1.0.0",
			"Writes proxy request/response logs to JSONL files",
		),
	}
}

// Initialize implements Plugin interface
func (p *FileProxyCollector) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	// Parse configuration
	outputDir := config["output_directory"]
	if outputDir == "" {
		outputDir = "./data/proxy_logs"
	}
	p.outputDir = outputDir

	// Check if enabled
	if enabled := config["enabled"]; enabled == "false" {
		p.enabled = false
	} else {
		p.enabled = true
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", p.outputDir, err)
	}

	// Log successful initialization
	ctx.Services.Logger().Info("FileProxyCollector initialized",
		"output_dir", p.outputDir,
		"enabled", p.enabled,
	)

	return nil
}

// HandleProxyLog implements DataCollector - this is the main functionality
func (p *FileProxyCollector) HandleProxyLog(ctx plugin_sdk.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	// Convert Unix timestamp to time.Time
	timestamp := time.Unix(req.Timestamp, 0)

	// Create log entry
	logEntry := map[string]interface{}{
		"timestamp":     timestamp.Format(time.RFC3339),
		"app_id":        req.AppId,
		"user_id":       req.UserId,
		"vendor":        req.Vendor,
		"response_code": req.ResponseCode,
		"request_id":    req.RequestId,
		"request_size":  len(req.RequestBody),
		"response_size": len(req.ResponseBody),
		// Include first 200 chars of request/response for debugging
		"request_preview":  truncateString(string(req.RequestBody), 200),
		"response_preview": truncateString(string(req.ResponseBody), 200),
	}

	// Add context information if available
	if req.Context != nil {
		logEntry["context"] = map[string]interface{}{
			"llm_id":   req.Context.LlmId,
			"llm_slug": req.Context.LlmSlug,
		}
	}

	// Convert to JSON
	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		ctx.Services.Logger().Error("Failed to marshal log entry", "error", err)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to marshal log entry: %v", err),
		}, nil
	}

	// Append to daily log file
	filename := fmt.Sprintf("proxy_logs_%s.jsonl", timestamp.Format("2006-01-02"))
	filePath := filepath.Join(p.outputDir, filename)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		ctx.Services.Logger().Error("Failed to open log file",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to open log file %s: %v", filePath, err),
		}, nil
	}
	defer file.Close()

	// Write JSON line
	if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
		ctx.Services.Logger().Error("Failed to write to log file",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write to log file: %v", err),
		}, nil
	}

	ctx.Services.Logger().Debug("Proxy log written",
		"file_path", filePath,
		"file_size", getFileSize(filePath),
	)

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]string{
			"file_path": filePath,
			"file_size": fmt.Sprintf("%d", getFileSize(filePath)),
		},
	}, nil
}

// HandleAnalytics implements DataCollector - not used by this plugin
func (p *FileProxyCollector) HandleAnalytics(ctx plugin_sdk.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles proxy logs
	return &pb.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleBudgetUsage implements DataCollector - not used by this plugin
func (p *FileProxyCollector) HandleBudgetUsage(ctx plugin_sdk.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles proxy logs
	return &pb.DataCollectionResponse{
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

func getFileSize(filePath string) int64 {
	if info, err := os.Stat(filePath); err == nil {
		return info.Size()
	}
	return 0
}

func main() {
	plugin := NewFileProxyCollector()
	plugin_sdk.Serve(plugin)
}
