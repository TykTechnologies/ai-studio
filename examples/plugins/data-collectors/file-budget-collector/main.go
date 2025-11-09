// plugins/examples/unified/data-collectors/file-budget-collector/main.go
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

// FileBudgetCollector implements DataCollector to append budget usage to text files
type FileBudgetCollector struct {
	plugin_sdk.BasePlugin
	outputDir     string
	enabled       bool
	csvMode       bool
	aggregateMode bool // If true, maintain running totals per app/period
}

// NewFileBudgetCollector creates a new instance of the plugin
func NewFileBudgetCollector() *FileBudgetCollector {
	return &FileBudgetCollector{
		BasePlugin: plugin_sdk.NewBasePlugin(
			"file-budget-collector",
			"1.0.0",
			"Writes budget usage data to files in CSV or JSONL format",
		),
	}
}

// Initialize implements Plugin interface
func (p *FileBudgetCollector) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	// Parse configuration
	outputDir := config["output_directory"]
	if outputDir == "" {
		outputDir = "./data/budget"
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

	// Check aggregate mode
	if aggregate := config["aggregate_mode"]; aggregate == "true" {
		p.aggregateMode = true
	} else {
		p.aggregateMode = false
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
	ctx.Services.Logger().Info("FileBudgetCollector initialized",
		"output_dir", p.outputDir,
		"format", p.getFormat(),
		"enabled", p.enabled,
		"aggregate_mode", p.aggregateMode,
	)

	return nil
}

// HandleProxyLog implements DataCollector - not used by this plugin
func (p *FileBudgetCollector) HandleProxyLog(ctx plugin_sdk.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles budget data
	return &pb.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleAnalytics implements DataCollector - not used by this plugin
func (p *FileBudgetCollector) HandleAnalytics(ctx plugin_sdk.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	// This plugin only handles budget data
	return &pb.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleBudgetUsage implements DataCollector - this is the main functionality
func (p *FileBudgetCollector) HandleBudgetUsage(ctx plugin_sdk.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	// Convert Unix timestamps to time.Time
	timestamp := time.Unix(req.Timestamp, 0)
	periodStart := time.Unix(req.PeriodStart, 0)
	periodEnd := time.Unix(req.PeriodEnd, 0)

	var filename, content string

	if p.csvMode {
		filename = fmt.Sprintf("budget_usage_%s.csv", timestamp.Format("2006-01-02"))
		content = fmt.Sprintf("%s,%d,%d,%d,%.6f,%d,%d,%d,%s,%s,%s\n",
			timestamp.Format(time.RFC3339),
			req.AppId,
			req.LlmId,
			req.TokensUsed,
			req.Cost,
			req.RequestsCount,
			req.PromptTokens,
			req.CompletionTokens,
			periodStart.Format(time.RFC3339),
			periodEnd.Format(time.RFC3339),
			req.RequestId,
		)
	} else {
		// JSON Lines format
		filename = fmt.Sprintf("budget_usage_%s.jsonl", timestamp.Format("2006-01-02"))

		logEntry := map[string]interface{}{
			"timestamp":         timestamp.Format(time.RFC3339),
			"app_id":           req.AppId,
			"llm_id":           req.LlmId,
			"tokens_used":      req.TokensUsed,
			"cost":             req.Cost,
			"requests_count":   req.RequestsCount,
			"prompt_tokens":    req.PromptTokens,
			"completion_tokens": req.CompletionTokens,
			"period_start":     periodStart.Format(time.RFC3339),
			"period_end":       periodEnd.Format(time.RFC3339),
			"request_id":       req.RequestId,
			"context": map[string]interface{}{
				"period_duration_days": int(periodEnd.Sub(periodStart).Hours() / 24),
			},
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			return &pb.DataCollectionResponse{
				Success:      false,
				Handled:      false,
				ErrorMessage: fmt.Sprintf("failed to marshal budget entry: %v", err),
			}, nil
		}
		content = string(jsonData) + "\n"
	}

	// Write to file
	filePath := filepath.Join(p.outputDir, filename)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		ctx.Services.Logger().Error("Failed to open budget file",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to open budget file %s: %v", filePath, err),
		}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		ctx.Services.Logger().Error("Failed to write budget data",
			"file_path", filePath,
			"error", err,
		)
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write budget data: %v", err),
		}, nil
	}

	// If in aggregate mode, also update summary file
	if p.aggregateMode {
		if err := p.updateAggregate(ctx, req, periodStart, periodEnd); err != nil {
			// Log error but don't fail the response
			ctx.Services.Logger().Warn("Failed to update aggregate file", "error", err)
		}
	}

	ctx.Services.Logger().Debug("Budget data written",
		"file_path", filePath,
		"format", p.getFormat(),
		"aggregate_mode", p.aggregateMode,
	)

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]string{
			"file_path":      filePath,
			"format":         p.getFormat(),
			"aggregate_mode": fmt.Sprintf("%t", p.aggregateMode),
		},
	}, nil
}

// ensureCSVHeader creates CSV header if file doesn't exist
func (p *FileBudgetCollector) ensureCSVHeader() error {
	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("budget_usage_%s.csv", today)
	filePath := filepath.Join(p.outputDir, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Create file with header
		file, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		header := "timestamp,app_id,llm_id,tokens_used,cost,requests_count,prompt_tokens,completion_tokens,period_start,period_end,request_id\n"
		_, err = file.WriteString(header)
		return err
	}

	return nil
}

// updateAggregate maintains running totals in a separate aggregate file
func (p *FileBudgetCollector) updateAggregate(ctx plugin_sdk.Context, req *pb.BudgetUsageRequest, periodStart, periodEnd time.Time) error {
	aggregateFile := filepath.Join(p.outputDir, "budget_aggregate.json")

	// Load existing aggregates
	aggregates := make(map[string]map[string]interface{})
	if data, err := os.ReadFile(aggregateFile); err == nil {
		json.Unmarshal(data, &aggregates)
	}

	// Create key for this app/llm/period combination
	key := fmt.Sprintf("app_%d_llm_%d_%s", req.AppId, req.LlmId, periodStart.Format("2006-01"))

	if _, exists := aggregates[key]; !exists {
		aggregates[key] = map[string]interface{}{
			"app_id":            req.AppId,
			"llm_id":            req.LlmId,
			"period_start":      periodStart.Format(time.RFC3339),
			"period_end":        periodEnd.Format(time.RFC3339),
			"total_tokens":      int64(0),
			"total_cost":        float64(0),
			"total_requests":    int64(0),
			"prompt_tokens":     int64(0),
			"completion_tokens": int64(0),
			"last_updated":      time.Now().Format(time.RFC3339),
		}
	}

	// Update aggregates
	agg := aggregates[key]
	agg["total_tokens"] = int64(agg["total_tokens"].(float64)) + req.TokensUsed
	agg["total_cost"] = agg["total_cost"].(float64) + req.Cost
	agg["total_requests"] = int64(agg["total_requests"].(float64)) + int64(req.RequestsCount)
	agg["prompt_tokens"] = int64(agg["prompt_tokens"].(float64)) + req.PromptTokens
	agg["completion_tokens"] = int64(agg["completion_tokens"].(float64)) + req.CompletionTokens
	agg["last_updated"] = time.Now().Format(time.RFC3339)

	// Write back to file
	data, err := json.MarshalIndent(aggregates, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(aggregateFile, data, 0644)
}

func (p *FileBudgetCollector) getFormat() string {
	if p.csvMode {
		return "csv"
	}
	return "jsonl"
}

func main() {
	plugin := NewFileBudgetCollector()
	plugin_sdk.Serve(plugin)
}
