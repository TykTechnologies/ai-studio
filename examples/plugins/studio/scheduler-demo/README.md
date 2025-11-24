# Scheduler Demo Plugin

This plugin demonstrates the **Scheduler capability** in AI Studio, showing how to implement periodic background tasks.

## Features

This plugin includes three scheduled tasks to demonstrate different patterns:

### 1. Minute Counter Task
- **Schedule**: Every minute (`* * * * *`)
- **Timezone**: UTC
- **Timeout**: 30 seconds
- **Purpose**: Demonstrates high-frequency tasks, increments a counter in KV storage

### 2. Hourly Data Cleanup
- **Schedule**: Every hour (`0 * * * *`)
- **Timezone**: UTC
- **Timeout**: 120 seconds
- **Purpose**: Demonstrates cleanup operations with configurable retention period

### 3. Daily Summary Report
- **Schedule**: Daily at 9 AM (`0 9 * * *`)
- **Timezone**: America/New_York
- **Timeout**: 60 seconds
- **Purpose**: Demonstrates timezone-aware scheduling and report generation

## How It Works

### 1. Plugin Implementation

The plugin implements the `SchedulerPlugin` capability:

```go
type SchedulerDemoPlugin struct {
    plugin_sdk.BasePlugin
}

func (p *SchedulerDemoPlugin) ExecuteScheduledTask(
    ctx plugin_sdk.Context,
    schedule *plugin_sdk.Schedule,
) error {
    // Route to specific handler based on schedule.ID
    switch schedule.ID {
    case "minute-task":
        return p.handleMinuteTask(ctx, schedule)
    // ... other handlers
    }
}
```

### 2. Manifest Configuration

Schedules are declared in `plugin.manifest.json`:

```json
{
  "schedules": [
    {
      "id": "minute-task",
      "name": "Minute Counter Task",
      "cron": "* * * * *",
      "timezone": "UTC",
      "enabled": true,
      "timeout_seconds": 30,
      "config": {
        "description": "Runs every minute"
      }
    }
  ]
}
```

### 3. Accessing Services

Scheduled tasks have full access to AI Studio services:

```go
func (p *SchedulerDemoPlugin) handleMinuteTask(
    ctx plugin_sdk.Context,
    schedule *plugin_sdk.Schedule,
) error {
    logger := ctx.Services.Logger()
    kv := ctx.Services.KV()

    // Use KV storage
    count, _ := kv.Read(ctx, "counter")

    // Use logging
    logger.Info("Task executed", "count", count)

    return nil
}
```

## Building and Installing

### Build the Plugin

```bash
cd examples/plugins/studio/scheduler-demo/server
go build -o scheduler-demo
```

### Install in AI Studio

1. **Via UI**: Upload the binary through the Plugins page
2. **Via Command**: Place the binary in your plugins directory and register it

### Verify Installation

Check the AI Studio logs for:
```
[Scheduler] Registered schedule 1 (Minute Counter Task) with cron '* * * * *'
[Scheduler] Registered schedule 2 (Hourly Data Cleanup) with cron '0 * * * *'
[Scheduler] Registered schedule 3 (Daily Summary Report) with cron '0 9 * * *'
```

## Monitoring Execution

### Via Logs

Watch for execution logs:
```
[Scheduler] Executing schedule 1 (Minute Counter Task) for plugin 42
🔔 Scheduled task executing schedule_id=minute-task
✅ Minute task completed execution_count=15
[Scheduler] Schedule 1 execution completed successfully (0.05s)
```

### Via Database

Query execution history:
```sql
SELECT
    ps.name,
    pse.status,
    pse.started_at,
    pse.duration,
    pse.error
FROM plugin_schedule_executions pse
JOIN plugin_schedules ps ON pse.schedule_id = ps.id
WHERE ps.plugin_id = <plugin_id>
ORDER BY pse.started_at DESC
LIMIT 50;
```

### Via KV Storage

Check task data:
```go
// Minute counter
count := kv.Read(ctx, "scheduler-demo:minute-count")

// Last execution timestamp
lastRun := kv.Read(ctx, "scheduler-demo:minute-last-run")

// Cleanup stats
stats := kv.Read(ctx, "scheduler-demo:cleanup-2025-01-15")

// Daily report
report := kv.Read(ctx, "scheduler-demo:report-2025-01-15")
```

## Cron Expression Reference

| Expression | Description |
|------------|-------------|
| `* * * * *` | Every minute |
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 */6 * * *` | Every 6 hours |
| `0 0 * * *` | Daily at midnight |
| `0 9 * * *` | Daily at 9 AM |
| `0 9 * * 1` | Every Monday at 9 AM |
| `0 0 1 * *` | First day of each month |

## Configuration Options

### Schedule Fields

- **id**: Unique identifier (used in ExecuteScheduledTask routing)
- **name**: Human-readable name (shown in logs and UI)
- **cron**: Cron expression (supports second precision)
- **timezone**: IANA timezone (e.g., "America/New_York", "UTC")
- **enabled**: Whether schedule is active (can be toggled via API/UI)
- **timeout_seconds**: Max execution time (default: 60)
- **config**: Custom JSON config passed to ExecuteScheduledTask

### Timeout Behavior

If a task exceeds its timeout:
- The task is marked as `timeout` in execution history
- An error is logged: "execution timed out after Ns"
- The plugin process is NOT killed (use timeouts for monitoring only)

### Overlap Prevention

The scheduler automatically prevents overlapping executions:
- Only one execution per schedule at a time
- If a task is still running when the next cron trigger fires, it's skipped
- A warning is logged: "Schedule X already running (execution Y), skipping"

## Best Practices

### 1. Task Design

✅ **DO:**
- Keep tasks short and focused
- Use KV storage for state persistence
- Handle errors gracefully (return error, don't panic)
- Log important events for debugging
- Use schedule.Config for flexibility

❌ **DON'T:**
- Run long-running operations without checkpoints
- Store sensitive data in KV without encryption
- Assume tasks run exactly on time (cron is best-effort)
- Use global state (tasks may run in different processes)

### 2. Error Handling

```go
func (p *MyPlugin) ExecuteScheduledTask(
    ctx plugin_sdk.Context,
    schedule *plugin_sdk.Schedule,
) error {
    logger := ctx.Services.Logger()

    // DO: Log context for debugging
    logger.Info("Task starting", "schedule_id", schedule.ID)

    // DO: Return errors (they're recorded in execution history)
    if err := doWork(); err != nil {
        logger.Error("Task failed", "error", err)
        return fmt.Errorf("work failed: %w", err)
    }

    // DO: Log success
    logger.Info("Task completed successfully")
    return nil
}
```

### 3. Testing Schedules

Use short intervals for testing:
```json
{
  "cron": "*/2 * * * *",  // Every 2 minutes (for testing)
  "timeout_seconds": 10
}
```

Then change to production schedule:
```json
{
  "cron": "0 0 * * *",    // Daily at midnight (production)
  "timeout_seconds": 300
}
```

## Troubleshooting

### Schedule Not Running

1. **Check plugin is active**:
   ```sql
   SELECT id, name, is_active FROM plugins WHERE name = 'Scheduler Demo Plugin';
   ```

2. **Check schedule is enabled**:
   ```sql
   SELECT * FROM plugin_schedules WHERE plugin_id = <plugin_id>;
   ```

3. **Check leader election**:
   ```sql
   SELECT * FROM scheduler_leases;
   ```
   - Verify `expires_at` is in the future
   - Only the leader runs schedules

4. **Check logs**:
   ```
   grep -i "scheduler" /var/log/ai-studio.log
   ```

### Task Failures

Check execution history:
```sql
SELECT
    started_at,
    status,
    duration,
    error
FROM plugin_schedule_executions
WHERE schedule_id = <schedule_id>
ORDER BY started_at DESC
LIMIT 10;
```

Common issues:
- **timeout**: Increase `timeout_seconds` in manifest
- **failed**: Check error message and plugin logs
- **No executions**: Plugin may not be loaded or schedule disabled

## Advanced Patterns

### Conditional Execution

```go
func (p *MyPlugin) ExecuteScheduledTask(
    ctx plugin_sdk.Context,
    schedule *plugin_sdk.Schedule,
) error {
    kv := ctx.Services.KV()

    // Check if execution is needed
    lastCheck, _ := kv.Read(ctx, "last-check")
    if shouldSkip(lastCheck) {
        return nil // Skip this execution
    }

    // Do work...
    return nil
}
```

### Distributed Coordination

```go
func (p *MyPlugin) handleBatchJob(
    ctx plugin_sdk.Context,
    schedule *plugin_sdk.Schedule,
) error {
    kv := ctx.Services.KV()

    // Use KV for distributed locking
    lockKey := "batch-job-lock"
    lockValue := fmt.Sprintf("%d", time.Now().Unix())

    // Try to acquire lock
    existing, _ := kv.Read(ctx, lockKey)
    if existing != "" {
        return fmt.Errorf("job already running")
    }

    // Set lock with TTL
    kv.WriteWithTTL(ctx, lockKey, lockValue, 10*time.Minute)
    defer kv.Delete(ctx, lockKey)

    // Do batch work...
    return nil
}
```

## Related Documentation

- [Plugin SDK Reference](../../../docs/plugin-sdk.md)
- [Scheduler Architecture](../../../docs/scheduler.md)
- [Service API Access](../../../docs/service-api.md)
- [KV Storage Guide](../../../docs/kv-storage.md)
