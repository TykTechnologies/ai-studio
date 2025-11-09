// plugins/examples/unified/data-collectors/file-analytics-collector/main.go
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

// FileAnalyticsCollector implements DataCollector to append analytics data to text files
type FileAnalyticsCollector struct {
	plugin_sdk.BasePlugin
	outputDir string
	enabled   bool
	csvMode   bool
}

// NewFileAnalyticsCollector creates a new instance of the plugin
func NewFileAnalyticsCollector() *FileAnalyticsCollector {
	return &FileAnalyticsCollector{
		BasePlugin: plugin_sdk.NewBasePlugin(
			"file-analytics-collector",
			"1.0.0",
			"Writes analytics data to files in CSV or JSONL format",
		),
	}
}

// Initialize implements Plugin interface
func (p *FileAnalyticsCollector) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	// Parse configuration
	outputDir := config["output_directory"]
	if outputDir == "" {
		outputDir = "./data/analytics"
	}
	p.outputDir = outputDir

	// Check if enabled
	if enabled := config["enabled"]; enabled == "false" {
		p.enabled = false
	} else {
		p.enabled = true
	}

	// Check output format
	if format := config["format"]; format == "csv" {
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

	// Log successful initialization
	ctx.Services.Logger().Info("FileAnalyticsCollector initialized",
		"output_dir", p.outputDir,
		"format", p.getFormat(),
		"enabled", p.enabled,
	)

	return nil
}

// HandleProxyLog implements DataCollector - not used by this plugin
func (p *FileAnalyticsCollector) HandleProxyLog(ctx plugin_sdk.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles analytics
	return &pb.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleAnalytics implements DataCollector - this is the main functionality
func (p *FileAnalyticsCollector) HandleAnalytics(ctx plugin_sdk.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	// Convert Unix timestamp to time.Time
	timestamp := time.Unix(req.Timestamp, 0)

	var filename, content string

	if p.csvMode {
		filename = fmt.Sprintf("analytics_%s.csv", timestamp.Format("2006-01-02"))
		content = fmt.Sprintf("%s,%d,%s,%s,%d,%d,%d,%d,%d,%.6f,%s,%d,%d,%d,%d,%s\n",
			timestamp.Format(time.RFC3339),
			req.LlmId,
			req.ModelName,
			req.Vendor,
			req.PromptTokens,
			req.ResponseTokens,
			req.CacheWritePromptTokens,
			req.CacheReadPromptTokens,
			req.TotalTokens,
			req.Cost,
			req.Currency,
			req.AppId,
			req.UserId,
			req.ToolCalls,
			req.Choices,
			req.RequestId,
		)
	} else {
		// JSON Lines format
		filename = fmt.Sprintf("analytics_%s.jsonl", timestamp.Format("2006-01-02"))

		logEntry := map[string]interface{}{
			"timestamp":                    timestamp.Format(time.RFC3339),
			"llm_id":                      req.LlmId,
			"model_name":                  req.ModelName,
			"vendor":                      req.Vendor,
			"prompt_tokens":               req.PromptTokens,
			"response_tokens":             req.ResponseTokens,
			"cache_write_prompt_tokens":   req.CacheWritePromptTokens,
			"cache_read_prompt_tokens":    req.CacheReadPromptTokens,
			"total_tokens":                req.TotalTokens,
			"cost":                        req.Cost,
			"currency":                    req.Currency,
			"app_id":                      req.AppId,
			"user_id":                     req.UserId,
			"tool_calls":                  req.ToolCalls,
			"choices":                     req.Choices,
			"request_id":                  req.RequestId,
		}

		// Add context information if available
		if req.Context != nil {
			logEntry["context"] = map[string]interface{}{
				"llm_slug": req.Context.LlmSlug,
			}
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			return &pb.DataCollectionResponse{
				Success:      false,
				Handled:      false,
				ErrorMessage: fmt.Sprintf("failed to marshal analytics entry: %v", err),
			}, nil
		}
		content = string(jsonData) + "\n"
	}

	// Write to file
	filePath := filepath.Join(p.outputDir, filename)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		ctx.Services.Logger().Error("Failed to open analytics file",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to open analytics file %s: %v", filePath, err),
		}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		ctx.Services.Logger().Error("Failed to write analytics data",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write analytics data: %v", err),
		}, nil
	}

	ctx.Services.Logger().Debug("Analytics data written",
		"file_path", filePath,
		"format", p.getFormat(),
	)

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]string{
			"file_path": filePath,
			"format":    p.getFormat(),
		},
	}, nil
}

// HandleBudgetUsage implements DataCollector - not used by this plugin
func (p *FileAnalyticsCollector) HandleBudgetUsage(ctx plugin_sdk.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles analytics
	return &pb.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// ensureCSVHeader creates CSV header if file doesn't exist
func (p *FileAnalyticsCollector) ensureCSVHeader() error {
	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("analytics_%s.csv", today)
	filePath := filepath.Join(p.outputDir, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Create file with header
		file, err := os.Create(filePath)
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

func (p *FileAnalyticsCollector) getFormat() string {
	if p.csvMode {
		return "csv"
	}
	return "jsonl"
}

func main() {
	plugin := NewFileAnalyticsCollector()
	plugin_sdk.Serve(plugin)
}
