# Telemetry System

## Introduction

The **Midsommar Telemetry System** provides anonymized usage statistics collection to help understand platform utilization and improve the product. Telemetry is **enabled by default** but can be easily disabled by users who prefer not to share usage data.

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Key Components](#key-components)
4. [Data Collection](#data-collection)
5. [Privacy Considerations](#privacy-considerations)
6. [Configuration](#configuration)
7. [Opting Out](#opting-out)

---

## Overview

The **Midsommar Telemetry System** objectives are:

- **Usage Analytics:** Collect anonymized statistics about platform usage patterns
- **Product Improvement:** Understand feature utilization to guide development priorities
- **Privacy-First:** No personally identifiable information (PII) is collected
- **Opt-Out Friendly:** Easy to disable for privacy-conscious users
- **Non-Intrusive:** Failures in telemetry never affect the main application functionality

---

## System Architecture

### Core Components:
- `TelemetryManager` ([services/telemetry_manager.go](../services/telemetry_manager.go)) - Central component that manages telemetry collection and transmission
- `TelemetryService` ([services/telemetry_service.go](../services/telemetry_service.go)) - Collects usage statistics from the database
- `AppConf.TelemetryEnabled` ([config/config.go](../config/config.go)) - Configuration setting to enable/disable telemetry

### Integration Points:
- **Application Startup** ([main.go](../main.go)) - Telemetry manager is initialized and started during application startup
- **Database** - Usage data is queried from existing database tables
- **HTTP Client** - Sends anonymized data to central telemetry service

### Data Flow:
1. Telemetry manager starts during application startup
2. Initial telemetry data is collected and sent immediately
3. Periodic collection occurs every hour (hardcoded)
4. Data is anonymized and sent to `https://telemetry.tyk.technology`
5. If telemetry is disabled, no data collection or transmission occurs

---

## Key Components

### 1. TelemetryManager

**File Location:** [services/telemetry_manager.go](../services/telemetry_manager.go)

The TelemetryManager is the central component of the telemetry system:

```go
type TelemetryManager struct {
    db               *gorm.DB
    telemetryService *TelemetryService
    enabled          bool
    version          string
    ctx              context.Context
    cancel           context.CancelFunc
}
```

Key methods:
- `Start()` - Initializes telemetry collection and starts periodic data transmission
- `Stop()` - Stops telemetry collection gracefully
- `collectAndSend()` - Gathers telemetry data and sends it to the telemetry service
- `generateInstanceID()` - Creates a consistent but anonymized instance identifier

### 2. TelemetryService

**File Location:** [services/telemetry_service.go](../services/telemetry_service.go)

The TelemetryService handles data collection from the database:

```go
type TelemetryService struct {
    DB *gorm.DB
}
```

Key methods:
- `GetLLMStats()` - Collects LLM usage statistics (model counts, token usage)
- `GetAppStats()` - Collects application statistics (app counts, proxy token usage)
- `GetUserStats()` - Collects user statistics (user counts by type, groups)
- `GetChatStats()` - Collects chat statistics (chat counts, interaction tokens)

### 3. TelemetryPayload

The structure of data sent to the telemetry service:

```go
type TelemetryPayload struct {
    Timestamp  time.Time              `json:"timestamp"`
    InstanceID string                 `json:"instance_id"`
    Version    string                 `json:"version"`
    LLMStats   map[string]interface{} `json:"llm_stats"`
    AppStats   map[string]interface{} `json:"app_stats"`
    UserStats  map[string]interface{} `json:"user_stats"`
    ChatStats  map[string]interface{} `json:"chat_stats"`
}
```

---

## Data Collection

### Collected Statistics:

1. **LLM Statistics:**
   - Total number of LLM configurations
   - Total token usage across all LLM interactions

2. **Application Statistics:**
   - Total number of applications
   - Total proxy token usage

3. **User Statistics:**
   - Total user count
   - Admin user count
   - Developer count
   - Chat user count
   - User group count

4. **Chat Statistics:**
   - Total chat count
   - Total chat interaction tokens

### Collection Process:
- Telemetry is collected **every hour** (hardcoded interval)
- Collection only occurs if telemetry is **enabled**
- Data is collected via SQL queries to existing database tables
- Individual collection failures are logged but don't stop the overall process

### Transmission:
- Data is sent to: `https://telemetry.tyk.technology` (hardcoded)
- Data is sent as JSON via HTTPS POST requests
- Instance ID is anonymized using SHA256 hashing
- Transmission failures are logged but don't affect application functionality

---

## Privacy Considerations

### No Personal Data:
- **No personally identifiable information (PII)** is collected
- **No user names, emails, or content** is transmitted
- **No API keys or credentials** are included
- Only **aggregate counts and statistics** are collected

### Anonymization:
- Instance IDs are generated using SHA256 hashing
- Daily rotation ensures instances cannot be tracked long-term
- Database connection info is hashed for consistent but anonymous identification

### Graceful Failures:
- Telemetry failures **never affect** the main application
- All telemetry errors are logged as warnings only
- Network issues, server unavailability, or data collection problems are handled gracefully

---

## Configuration

### Environment Variables:
- `TELEMETRY_ENABLED` - Set to "false" or "0" to disable telemetry collection (default: enabled)

### Hardcoded Settings:
- **Telemetry URL:** `https://telemetry.tyk.technology` 
- **Collection Period:** 1 hour
- **Timeout:** 30 seconds for HTTP requests

### Configuration in [config/config.go](../config/config.go):
```go
type AppConf struct {
    // Other fields...
    TelemetryEnabled bool // From TELEMETRY_ENABLED environment variable
}
```

### Default Behavior:
- **Telemetry is enabled by default**
- Users must explicitly opt-out by setting `TELEMETRY_ENABLED=false`
- When disabled, no data collection or network requests occur

---

## Opting Out

### How to Disable Telemetry:

**Option 1: Environment Variable**
```bash
export TELEMETRY_ENABLED=false
```

**Option 2: In .env file**
```
TELEMETRY_ENABLED=false
```

**Option 3: Docker/Container Environment**
```bash
docker run -e TELEMETRY_ENABLED=false your-image
```

### Verification:
When telemetry is disabled, you'll see this log message at startup:
```
Telemetry is disabled
```

When telemetry is enabled, you'll see these log messages:
```
Telemetry collection started - collecting usage statistics every 1h0m0s
Telemetry data will be sent to: https://telemetry.tyk.technology
To disable telemetry, set environment variable: TELEMETRY_ENABLED=false
```

---

## Integration

### Startup Integration:
The telemetry system is initialized in [main.go](../main.go) during application startup:

```go
// Initialize and start telemetry
telemetryManager := services.NewTelemetryManager(db, appConf.TelemetryEnabled, VERSION)
telemetryManager.Start()
defer telemetryManager.Stop()
```

### Graceful Shutdown:
The telemetry manager is properly stopped when the application shuts down, ensuring no background goroutines are left running.

---

## Technical Details

### Constants:
- `TelemetryURL = "https://telemetry.tyk.technology"`
- `TelemetryPeriod = time.Hour`

### HTTP Client Configuration:
- 30-second timeout for telemetry requests
- User-Agent header includes application version
- Content-Type: `application/json`

### Error Handling:
- All telemetry errors are logged as warnings
- Network failures don't retry (will attempt again in next cycle)
- JSON marshaling failures are logged
- HTTP errors (non-2xx status codes) are logged

This telemetry system provides valuable usage insights while respecting user privacy and maintaining application stability.
