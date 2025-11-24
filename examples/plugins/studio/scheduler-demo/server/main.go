package main

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
)

//go:embed plugin.manifest.json
var manifestJSON []byte

// SchedulerDemoPlugin demonstrates the scheduler capability
type SchedulerDemoPlugin struct {
	plugin_sdk.BasePlugin
}

// Initialize sets up the plugin and creates API-managed schedules
func (p *SchedulerDemoPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	// For AI Studio runtime, create API-managed schedules
	if ctx.Runtime == plugin_sdk.RuntimeStudio {
		if err := p.setupAPISchedules(ctx); err != nil {
			return fmt.Errorf("failed to setup API schedules: %w", err)
		}
	}

	return nil
}

// setupAPISchedules creates schedules via the service API
func (p *SchedulerDemoPlugin) setupAPISchedules(ctx plugin_sdk.Context) error {
	apiCtx := context.Background()

	// Check if minute-task schedule already exists
	if _, err := ai_studio_sdk.GetSchedule(apiCtx, "minute-task"); err != nil {
		// Create minute-task schedule
		minuteConfig := map[string]interface{}{
			"description": "Runs every minute to increment a counter (created via API)",
		}
		if _, err := ai_studio_sdk.CreateSchedule(
			apiCtx,
			"minute-task",
			"Minute Counter Task",
			"* * * * *",
			"UTC",
			30,
			minuteConfig,
			true,
		); err != nil {
			return fmt.Errorf("failed to create minute-task schedule: %w", err)
		}
	}

	// Check if hourly-cleanup schedule already exists
	if _, err := ai_studio_sdk.GetSchedule(apiCtx, "hourly-cleanup"); err != nil {
		// Create hourly-cleanup schedule
		cleanupConfig := map[string]interface{}{
			"retention_days": 7,
			"description":    "Cleans up old data every hour (created via API)",
		}
		if _, err := ai_studio_sdk.CreateSchedule(
			apiCtx,
			"hourly-cleanup",
			"Hourly Data Cleanup",
			"0 * * * *",
			"UTC",
			120,
			cleanupConfig,
			true,
		); err != nil {
			return fmt.Errorf("failed to create hourly-cleanup schedule: %w", err)
		}
	}

	return nil
}

// ExecuteScheduledTask implements the SchedulerPlugin capability
func (p *SchedulerDemoPlugin) ExecuteScheduledTask(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
	logger := ctx.Services.Logger()

	logger.Info("🔔 Scheduled task executing",
		"schedule_id", schedule.ID,
		"schedule_name", schedule.Name,
		"cron", schedule.Cron,
		"timezone", schedule.Timezone,
	)

	// Route to specific task handler based on schedule ID
	switch schedule.ID {
	case "minute-task":
		return p.handleMinuteTask(ctx, schedule)
	case "hourly-cleanup":
		return p.handleHourlyCleanup(ctx, schedule)
	case "daily-report":
		return p.handleDailyReport(ctx, schedule)
	default:
		return fmt.Errorf("unknown schedule ID: %s", schedule.ID)
	}
}

// handleMinuteTask runs every minute and increments a counter
func (p *SchedulerDemoPlugin) handleMinuteTask(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
	logger := ctx.Services.Logger()
	kv := ctx.Services.KV()

	// Read current count
	countKey := "scheduler-demo:minute-count"
	countBytes, err := kv.Read(ctx.Context, countKey)

	count := 0
	if err == nil && len(countBytes) > 0 {
		fmt.Sscanf(string(countBytes), "%d", &count)
	}

	// Increment
	count++

	// Store with timestamp
	timestamp := time.Now().Format(time.RFC3339)
	lastRunKey := "scheduler-demo:minute-last-run"

	if _, err := kv.Write(ctx.Context, countKey, []byte(fmt.Sprintf("%d", count)), nil); err != nil {
		return fmt.Errorf("failed to write count: %w", err)
	}

	if _, err := kv.Write(ctx.Context, lastRunKey, []byte(timestamp), nil); err != nil {
		return fmt.Errorf("failed to write timestamp: %w", err)
	}

	logger.Info("✅ Minute task completed",
		"execution_count", count,
		"timestamp", timestamp,
	)

	return nil
}

// handleHourlyCleanup demonstrates a cleanup task with config
func (p *SchedulerDemoPlugin) handleHourlyCleanup(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
	logger := ctx.Services.Logger()
	kv := ctx.Services.KV()

	// Get retention days from schedule config
	retentionDays := 7 // default
	if days, ok := schedule.Config["retention_days"].(float64); ok {
		retentionDays = int(days)
	}

	logger.Info("🧹 Running hourly cleanup",
		"retention_days", retentionDays,
	)

	// List all keys with our prefix
	keys, err := kv.List(ctx.Context, "scheduler-demo:")
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	cleanedCount := 0
	cutoffTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	for _, key := range keys {
		// Skip counter keys
		if key == "scheduler-demo:minute-count" || key == "scheduler-demo:minute-last-run" {
			continue
		}

		// Check if key is old (this is simplified - real implementation would check timestamps)
		valueBytes, err := kv.Read(ctx.Context, key)
		if err != nil {
			continue
		}

		// Try to parse as timestamp
		if timestamp, err := time.Parse(time.RFC3339, string(valueBytes)); err == nil {
			if timestamp.Before(cutoffTime) {
				if _, err := kv.Delete(ctx.Context, key); err == nil {
					cleanedCount++
				}
			}
		}
	}

	// Record cleanup stats
	statsKey := fmt.Sprintf("scheduler-demo:cleanup-%s", time.Now().Format("2006-01-02"))
	statsValue := []byte(fmt.Sprintf("cleaned:%d,checked:%d", cleanedCount, len(keys)))
	kv.WriteWithTTL(ctx.Context, statsKey, statsValue, 30*24*time.Hour) // 30 days retention

	logger.Info("✅ Cleanup completed",
		"keys_checked", len(keys),
		"keys_cleaned", cleanedCount,
	)

	return nil
}

// handleDailyReport generates a daily summary
func (p *SchedulerDemoPlugin) handleDailyReport(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
	logger := ctx.Services.Logger()
	kv := ctx.Services.KV()

	logger.Info("📊 Generating daily report")

	// Read statistics
	countKey := "scheduler-demo:minute-count"
	countBytes, _ := kv.Read(ctx.Context, countKey)
	countStr := string(countBytes)

	lastRunKey := "scheduler-demo:minute-last-run"
	lastRunBytes, _ := kv.Read(ctx.Context, lastRunKey)
	lastRun := string(lastRunBytes)

	// List cleanup history
	keys, _ := kv.List(ctx.Context, "scheduler-demo:cleanup-")
	recentCleanups := len(keys)

	// Generate report
	report := fmt.Sprintf(`
=== Scheduler Demo Daily Report ===
Date: %s
Total minute task executions: %s
Last execution: %s
Recent cleanups: %d
===================================
`, time.Now().Format("2006-01-02"), countStr, lastRun, recentCleanups)

	// Store report
	reportKey := fmt.Sprintf("scheduler-demo:report-%s", time.Now().Format("2006-01-02"))
	if _, err := kv.WriteWithTTL(ctx.Context, reportKey, []byte(report), 90*24*time.Hour); err != nil {
		return fmt.Errorf("failed to store report: %w", err)
	}

	logger.Info("✅ Daily report generated",
		"executions", countStr,
		"recent_cleanups", recentCleanups,
	)

	// Print report to logs
	fmt.Println(report)

	return nil
}

// GetManifest returns the plugin manifest
func (p *SchedulerDemoPlugin) GetManifest() ([]byte, error) {
	return manifestJSON, nil
}

// Shutdown cleans up resources
func (p *SchedulerDemoPlugin) Shutdown(ctx plugin_sdk.Context) error {
	logger := ctx.Services.Logger()
	logger.Info("Scheduler Demo Plugin shutting down")
	return nil
}

func main() {
	plugin_sdk.Serve(&SchedulerDemoPlugin{})
}
