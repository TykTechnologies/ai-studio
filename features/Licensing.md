# Licensing System (ENTERPRISE EDITION ONLY)

## Introduction

**NOTE: Licensing is an Enterprise Edition feature. Community Edition has no licensing requirements - all features are available by default.**

The Enterprise Edition licensing system provides JWT-based license validation, periodic license checks, feature entitlements, and usage telemetry. This document describes how the licensing system works in the Enterprise Edition.

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

## Edition Comparison

### Community Edition
- ✅ No license required
- ✅ All features available by default
- ✅ No periodic validation checks
- ✅ Optional telemetry (via `TELEMETRY_ENABLED`)
- ✅ No license expiry concerns

### Enterprise Edition
- ✅ JWT-based license validation
- ✅ License check at boot (exits if invalid)
- ✅ Periodic re-validation every 24 hours (configurable)
- ✅ Exits with fatal error if license invalid/expired
- ✅ Usage telemetry collection and transmission
- ✅ Feature entitlements (future use)

---

## Overview

The **Enterprise Edition Licensing System** provides a framework for validating licenses, controlling feature access, and collecting usage telemetry. Its core objectives are:

- **License Validation:** Verify license authenticity using JWT-based signatures and enforce expiration dates
- **Boot-Time Check:** Validate license at startup - process exits if invalid or expired
- **Periodic Verification:** Re-validate every 24 hours - process exits if validation fails
- **Usage Telemetry:** Collect anonymized usage statistics to understand platform utilization
- **Security:** Protect sensitive license information with RSA signature verification
- **Privacy:** License keys are hashed (SHA256) before transmission

---

## System Architecture

1. **Public Interface (Community + Enterprise):**
   - `licensing.Service` ([services/licensing/interface.go](../services/licensing/interface.go)) - License service interface
   - `licensing.Config` ([services/licensing/types.go](../services/licensing/types.go)) - Configuration types
   - `licensing.Factory` ([services/licensing/factory.go](../services/licensing/factory.go)) - Factory pattern for edition selection
   - `Community Stub` ([services/licensing/community.go](../services/licensing/community.go)) - CE implementation (always valid)

2. **Enterprise Implementation (Private Submodule):**
   - `EnterpriseService` ([enterprise/features/licensing/service.go](../enterprise/features/licensing/service.go)) - JWT validation and periodic checks
   - `TelemetryClient` ([enterprise/features/licensing/telemetry.go](../enterprise/features/licensing/telemetry.go)) - Stats collection and transmission
   - `Middleware` ([enterprise/features/licensing/middleware.go](../enterprise/features/licensing/middleware.go)) - API action tracking
   - `RSA Public Key` ([enterprise/features/licensing/pubkey.go](../enterprise/features/licensing/pubkey.go)) - JWT signature verification

3. **Integration Points:**
   - **main.go** - Initializes licensing service at boot
   - **api.go** - Adds telemetry middleware for request tracking
   - **config.go** - License configuration from environment variables
   - **main_enterprise.go** - Conditional import with build tag

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
   - Data is sent to a central telemetry service (default: https://telemetry.tyk.technology)
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

### Enterprise Edition Only

1. **Required Environment Variable:**
   ```bash
   TYK_AI_LICENSE=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
   ```
   - **REQUIRED** for Enterprise Edition
   - Process will exit at boot if missing or invalid
   - Must be a valid JWT token signed with the enterprise RSA private key

2. **Optional Environment Variables:**
   ```bash
   # Telemetry configuration
   LICENSE_TELEMETRY_URL=https://telemetry.tyk.technology/api/track  # Default endpoint
   LICENSE_TELEMETRY_PERIOD=1h                  # How often to send telemetry (default: 1h)
   LICENSE_VALIDITY_CHECK_PERIOD=24h            # How often to re-validate license (default: 24h)
   LICENSE_DISABLE_TELEMETRY=false              # Set to true to disable telemetry
   LICENSE_TELEMETRY_CONCURRENCY=20             # Max concurrent telemetry requests (default: 20)
   ```

3. **Configuration in [config/config.go](../config/config.go):**
   ```go
   type AppConf struct {
       // Enterprise Edition licensing
       LicenseKey                  string        // From TYK_AI_LICENSE
       LicenseTelemetryPeriod      time.Duration // From LICENSE_TELEMETRY_PERIOD (default: 1h)
       LicenseDisableTelemetry     bool          // From LICENSE_DISABLE_TELEMETRY (default: false)
       LicenseTelemetryURL         string        // From LICENSE_TELEMETRY_URL
       LicenseValidityPeriod       time.Duration // From LICENSE_VALIDITY_CHECK_PERIOD (default: 24h)
       LicenseTelemetryConcurrency int           // From LICENSE_TELEMETRY_CONCURRENCY (default: 20)
   }
   ```

4. **Default Values:**
   - `LicenseKey` - No default, REQUIRED for ENT
   - `LicenseTelemetryURL` - `https://telemetry.tyk.technology/api/track`
   - `LicenseTelemetryPeriod` - `1h`
   - `LicenseValidityPeriod` - `24h`
   - `LicenseDisableTelemetry` - `false`
   - `LicenseTelemetryConcurrency` - `20`

### Community Edition
- No licensing configuration required
- All license environment variables are ignored
   - `LicenseTelemetryPeriod` - Default: 1 hour
   - `LicenseDisableTelemetry` - Default: false
   - `LicenseTelemetryURL` - Default: https://telemetry.tyk.technology
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
