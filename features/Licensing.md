# Licensing System (REMOVED)

## Introduction

**NOTE: The licensing system has been removed from the codebase. All features are now available by default without requiring a license.**

This document is kept for historical reference only. The previous licensing system included feature control, license validation, and usage telemetry.

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Key Components](#key-components)
4. [License Validation](#license-validation)
5. [Feature Entitlements](#feature-entitlements)
6. [Telemetry System](#telemetry-system)
7. [API Integration](#api-integration)
8. [Configuration Requirements](#configuration-requirements)
9. [Potential Enhancements](#potential-enhancements)

---

## Overview

The **Midsommar Licensing System** provides a framework for validating licenses, controlling feature access, and collecting usage telemetry. Its core objectives are:

- **License Validation:** Verify license authenticity using JWT-based signatures and enforce expiration dates
- **Feature Control:** Enable or disable specific platform features based on license entitlements
- **Usage Telemetry:** Collect anonymized usage statistics to understand platform utilization
- **Security:** Protect sensitive license information and ensure secure validation
- **Periodic Verification:** Regularly check license validity to ensure continued compliance
- **Integration:** Seamlessly integrate with other Midsommar components to enforce licensing constraints

---

## System Architecture

1. **Core Components:**
   - `Licenser` ([licensing/licensing.go](../licensing/licensing.go)) - Central component that manages license validation, feature access, and telemetry
   - `LicenseInfo` ([licensing/types.go](../licensing/types.go)) - Represents the parsed license with features and metadata
   - `Feature` ([licensing/features.go](../licensing/features.go)) - Represents individual feature entitlements with typed values
   - `Client` ([licensing/telemetry.go](../licensing/telemetry.go)) - Handles telemetry data collection and transmission
   - `TelemetryService` ([services/telemetry_service.go](../services/telemetry_service.go)) - Collects usage statistics from the database

2. **Integration Points:**
   - **API Middleware** ([licensing/middleware.go](../licensing/middleware.go)) - Integrates with the API to track usage and enforce license constraints
   - **Configuration** ([config/config.go](../config/config.go)) - Provides license key and telemetry settings
   - **Database** - Stores usage data that feeds into telemetry reports

3. **Data Flow:**
   - License validation occurs at startup and periodically during operation
   - Feature access is checked via the `Entitlement` method when features are used
   - Telemetry data is collected periodically and sent to a central telemetry service
   - API requests are tracked through middleware for usage analysis

---

## Key Components

### 1. Licenser

**File Location:** [licensing/licensing.go](../licensing/licensing.go)

The Licenser is the central component of the licensing system:

```go
type Licenser struct {
    license         *LicenseInfo
    config          LicenseConfig
    telemetryClient *Client
    done            chan bool
    lock            sync.RWMutex
    featuresInit    chan struct{}
    initialized     bool
}
```

Key methods:
- `Start()` - Initializes license validation and starts periodic checks and telemetry
- `Stop()` - Stops periodic checks and telemetry
- `FeatureSet()` - Returns all available features
- `Entitlement(name string)` - Checks if a specific feature is available
- `isLicensed()` - Validates the license key
- `SendTelemetry()` - Collects and sends usage statistics

### 2. LicenseInfo

**File Location:** [licensing/types.go](../licensing/types.go)

The LicenseInfo structure represents a parsed and validated license:

```go
type LicenseInfo struct {
    Key       string
    IsValid   bool
    ExpiresAt time.Time
    Version   string
    Features  map[string]*Feature
    claims    jwt.MapClaims
}
```

Key methods:
- `setup()` - Initializes license information from JWT claims
- `setVersion()` - Extracts version information from claims
- `setLicenseExpire()` - Sets expiration date from claims
- `setFeatures()` - Parses feature entitlements from claims

### 3. Feature

**File Location:** [licensing/features.go](../licensing/features.go)

The Feature structure represents individual feature entitlements:

```go
type Feature struct {
    tp        string
    valBool   bool
    valString string
    valInt    int
}
```

Key methods:
- `Bool()` - Returns the boolean value of the feature
- `String()` - Returns the string value of the feature
- `Int()` - Returns the integer value of the feature

### 4. Telemetry Client

**File Location:** [licensing/telemetry.go](../licensing/telemetry.go)

The Client handles telemetry data transmission:

```go
type Client struct {
    http *http.Client
    URL  string
}

type Event struct {
    Identity   string                 `json:"identity"`
    Event      string                 `json:"event"`
    Timestamp  int64                  `json:"timestamp"`
    Properties map[string]interface{} `json:"properties,omitempty"`
}
```

Key methods:
- `Track(identity, eventName string, properties map[string]interface{})` - Sends telemetry events

---

## License Validation

The licensing system uses JWT (JSON Web Tokens) with RSA signatures for secure license validation:

1. **Initialization:**
   - The license key is provided via the `TYK_AI_LICENSE` environment variable
   - The system validates the signature using a built-in public key
   - Claims are extracted and used to populate the `LicenseInfo` structure

2. **Validation Process:**
   - The JWT signature is verified using RSA-256
   - The expiration date is checked to ensure the license is still valid
   - Feature entitlements are extracted from the `scope` claim

3. **Periodic Validation:**
   - The license is re-validated periodically (default: every 10 minutes)
   - If validation fails, the application will terminate with a fatal error

4. **JWT Structure:**
   - Standard JWT claims (`exp`, `iat`, `nbf`) for time-based validation
   - Custom `scope` claim containing comma-separated feature entitlements
   - Custom `v` claim for license version information

---

## Feature Entitlements

The licensing system controls access to features through entitlements:

1. **Defined Features:**
   - `feature_portal` - Controls access to the portal interface
   - `feature_chat` - Controls access to the chat interface
   - `feature_gateway` - Controls access to the gateway functionality
   - `track` - Controls whether telemetry is enabled

2. **Feature Types:**
   - Boolean features (true/false)
   - String features (text values)
   - Integer features (numeric values)

3. **Checking Entitlements:**
   - Code calls `licenser.Entitlement("feature_name")` to check if a feature is available
   - Returns both the feature object and a boolean indicating if it exists
   - Feature values can be accessed via `Bool()`, `String()`, or `Int()` methods

4. **Default Behavior:**
   - If a feature is not present in the license, `Entitlement()` returns `false`
   - If the license is invalid or missing, no features are available

---

## Telemetry System

The telemetry system collects anonymized usage statistics:

1. **Data Collection:**
   - **LLM Statistics:** Count of LLMs and total tokens processed
   - **App Statistics:** Count of apps and proxy tokens
   - **User Statistics:** Counts of users, admins, developers, and groups
   - **Chat Statistics:** Count of chats and chat tokens

2. **Collection Process:**
   - Telemetry is collected periodically (default: every hour)
   - Collection only occurs if the `track` feature is enabled
   - Data is collected via the `TelemetryService` which queries the database

3. **Transmission:**
   - Data is sent to a central telemetry service (default: https://telemetry.tyk.technologies)
   - The license key is hashed for anonymization before transmission
   - Data is sent as JSON via HTTPS POST requests

4. **Privacy Considerations:**
   - No personally identifiable information is collected
   - Only aggregate counts and statistics are transmitted
   - Telemetry can be disabled via configuration

5. **HTTP Telemetry:**
   - API requests can be tracked via middleware
   - Records action, status code, and access type (UI or API)
   - Helps understand platform usage patterns

---

## API Integration

The licensing system integrates with the API through middleware:

1. **Telemetry Middleware:**
   - Tracks API requests for telemetry purposes
   - Records the action, status code, and access type
   - Only active when telemetry is enabled

2. **Action Tracking:**
   - Actions can be set on API handlers using `SetAction` or `ActionHandler`
   - Actions are stored in the Gin context and extracted after request processing
   - Helps categorize API usage for analysis

3. **Implementation:**
   ```go
   func (l *Licenser) TelemetryMiddleware() gin.HandlerFunc {
       return func(c *gin.Context) {
           if !l.TelemetryEnabled() {
               c.Next()
               return
           }

           c.Next()

           action, exists := c.Get(CtxActionKey)

           if exists {
               if actionStr, ok := action.(string); ok && actionStr != "" {
                   accessType := "ui"
                   if c.GetHeader("Authorization") != "" {
                       accessType = "api"
                   }

                   l.SendHTTPTelemetry(actionStr, c.Writer.Status(), accessType)
               }
           }
       }
   }
   ```

---

## Configuration Requirements

1. **Environment Variables:**
   - `TYK_AI_LICENSE` - The license key (JWT token) for the application
   - `LICENSE_DISABLE_TELEMETRY` - Set to "true" or "1" to disable telemetry collection
   - `LICENSE_TELEMETRY_URL` - Custom URL for the telemetry service (default: https://telemetry.tyk.technologies)
   - `LICENSE_TELEMETRY_PERIOD` - Custom duration for telemetry collection interval (e.g., "1h", "30m")

2. **Configuration in [config/config.go](../config/config.go):**
   ```go
   type AppConf struct {
       // Other fields...
       LicenseKey              string        // From TYK_AI_LICENSE
       LicenseTelemetryPeriod  time.Duration // From LICENSE_TELEMETRY_PERIOD
       LicenseDisableTelemetry bool          // From LICENSE_DISABLE_TELEMETRY
       LicenseTelemetryURL     string        // From LICENSE_TELEMETRY_URL
   }
   ```

3. **Default Values:**
   - `LicenseKey` - No default, must be provided
   - `LicenseTelemetryPeriod` - Default: 1 hour
   - `LicenseDisableTelemetry` - Default: false
   - `LicenseTelemetryURL` - Default: https://telemetry.tyk.technologies
   - `ValidityCheckPeriod` - Default: 10 minutes (not configurable via environment variables)

4. **License Requirements:**
   - Format: JWT token signed with RSA-256
   - Must contain valid expiration date
   - Must contain feature entitlements in the `scope` claim
   - Absence will cause the application to terminate with a fatal error

---

## Potential Enhancements

1. **License Management UI:** Add a simple admin interface to view license status and features.

2. **Graceful Degradation:** Implement a more user-friendly approach when license validation fails.

3. **Additional Telemetry Metrics:** Expand the telemetry system to collect more usage statistics.
