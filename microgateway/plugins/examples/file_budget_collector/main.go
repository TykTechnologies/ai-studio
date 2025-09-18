// plugins/examples/file_budget_collector/main.go
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

// FileBudgetCollector implements DataCollectionPlugin to append budget usage to text files
type FileBudgetCollector struct {
	outputDir     string
	enabled       bool
	csvMode       bool
	aggregateMode bool // If true, maintain running totals per app/period
}

// Initialize implements BasePlugin
func (p *FileBudgetCollector) Initialize(config map[string]interface{}) error {
	// Parse configuration
	outputDir, ok := config["output_directory"].(string)
	if !ok || outputDir == "" {
		outputDir = "./data/budget"
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

	// Check aggregate mode
	if aggregate, ok := config["aggregate_mode"].(bool); ok {
		p.aggregateMode = aggregate
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

	return nil
}

// GetHookType implements BasePlugin
func (p *FileBudgetCollector) GetHookType() sdk.HookType {
	return sdk.HookTypeDataCollection
}

// GetName implements BasePlugin
func (p *FileBudgetCollector) GetName() string {
	return "file-budget-collector"
}

// GetVersion implements BasePlugin
func (p *FileBudgetCollector) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *FileBudgetCollector) Shutdown() error {
	return nil
}

// HandleProxyLog implements DataCollectionPlugin - not used by this plugin
func (p *FileBudgetCollector) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles budget data
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleAnalytics implements DataCollectionPlugin - not used by this plugin
func (p *FileBudgetCollector) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	// This plugin only handles budget data
	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: false,
	}, nil
}

// HandleBudgetUsage implements DataCollectionPlugin - this is the main functionality
func (p *FileBudgetCollector) HandleBudgetUsage(ctx context.Context, req *sdk.BudgetUsageData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
	if !p.enabled {
		return &sdk.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	var filename, content string
	var err error

	if p.csvMode {
		filename = fmt.Sprintf("budget_usage_%s.csv", req.Timestamp.Format("2006-01-02"))
		content = fmt.Sprintf("%s,%d,%d,%d,%.6f,%d,%d,%d,%s,%s,%s\n",
			req.Timestamp.Format(time.RFC3339),
			req.AppID,
			req.LLMID,
			req.TokensUsed,
			req.Cost,
			req.RequestsCount,
			req.PromptTokens,
			req.CompletionTokens,
			req.PeriodStart.Format(time.RFC3339),
			req.PeriodEnd.Format(time.RFC3339),
			req.RequestID,
		)
	} else {
		// JSON Lines format
		filename = fmt.Sprintf("budget_usage_%s.jsonl", req.Timestamp.Format("2006-01-02"))
		
		logEntry := map[string]interface{}{
			"timestamp":         req.Timestamp.Format(time.RFC3339),
			"app_id":           req.AppID,
			"llm_id":           req.LLMID,
			"tokens_used":      req.TokensUsed,
			"cost":             req.Cost,
			"requests_count":   req.RequestsCount,
			"prompt_tokens":    req.PromptTokens,
			"completion_tokens": req.CompletionTokens,
			"period_start":     req.PeriodStart.Format(time.RFC3339),
			"period_end":       req.PeriodEnd.Format(time.RFC3339),
			"request_id":       req.RequestID,
			"context": map[string]interface{}{
				"period_duration_days": int(req.PeriodEnd.Sub(req.PeriodStart).Hours() / 24),
			},
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			return &sdk.DataCollectionResponse{
				Success:      false,
				Handled:      false,
				ErrorMessage: fmt.Sprintf("failed to marshal budget entry: %v", err),
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
			ErrorMessage: fmt.Sprintf("failed to open budget file %s: %v", filepath, err),
		}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return &sdk.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: fmt.Sprintf("failed to write budget data: %v", err),
		}, nil
	}

	// If in aggregate mode, also update summary file
	if p.aggregateMode {
		if err := p.updateAggregate(req); err != nil {
			// Log error but don't fail the response
			fmt.Printf("Warning: failed to update aggregate file: %v\n", err)
		}
	}

	return &sdk.DataCollectionResponse{
		Success: true,
		Handled: true,
		Metadata: map[string]interface{}{
			"file_path":      filepath,
			"format":         getOutputFormat(p.csvMode),
			"aggregate_mode": p.aggregateMode,
		},
	}, nil
}

// ensureCSVHeader creates CSV header if file doesn't exist
func (p *FileBudgetCollector) ensureCSVHeader() error {
	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("budget_usage_%s.csv", today)
	filepath := filepath.Join(p.outputDir, filename)

	// Check if file exists
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		// Create file with header
		file, err := os.Create(filepath)
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
func (p *FileBudgetCollector) updateAggregate(req *sdk.BudgetUsageData) error {
	aggregateFile := filepath.Join(p.outputDir, "budget_aggregate.json")
	
	// Load existing aggregates
	aggregates := make(map[string]map[string]interface{})
	if data, err := os.ReadFile(aggregateFile); err == nil {
		json.Unmarshal(data, &aggregates)
	}
	
	// Create key for this app/llm/period combination
	key := fmt.Sprintf("app_%d_llm_%d_%s", req.AppID, req.LLMID, req.PeriodStart.Format("2006-01"))
	
	if _, exists := aggregates[key]; !exists {
		aggregates[key] = map[string]interface{}{
			"app_id":            req.AppID,
			"llm_id":            req.LLMID,
			"period_start":      req.PeriodStart.Format(time.RFC3339),
			"period_end":        req.PeriodEnd.Format(time.RFC3339),
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
	agg["total_tokens"] = agg["total_tokens"].(int64) + req.TokensUsed
	agg["total_cost"] = agg["total_cost"].(float64) + req.Cost
	agg["total_requests"] = agg["total_requests"].(int64) + int64(req.RequestsCount)
	agg["prompt_tokens"] = agg["prompt_tokens"].(int64) + req.PromptTokens
	agg["completion_tokens"] = agg["completion_tokens"].(int64) + req.CompletionTokens
	agg["last_updated"] = time.Now().Format(time.RFC3339)
	
	// Write back to file
	data, err := json.MarshalIndent(aggregates, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(aggregateFile, data, 0644)
}

func getOutputFormat(csvMode bool) string {
	if csvMode {
		return "csv"
	}
	return "jsonl"
}

func main() {
	plugin := &FileBudgetCollector{}
	sdk.ServePlugin(plugin)
}