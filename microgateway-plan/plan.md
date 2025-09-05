# **Comprehensive Microgateway Implementation Plan**

## **Phase 1: Project Setup and Core Structure**

### **1.1 Create Project Directory Structure**

Create a new directory `microgateway` at the root level of the midsommar-clean project:

```bash
mkdir -p microgateway/{cmd/microgateway,internal/{config,database,services,auth,api,server},configs,deployments,scripts,tests}
```

### **1.2 Initialize Go Module**

```bash
cd microgateway
go mod init github.com/TykTechnologies/midsommar/microgateway
```

### **1.3 Project File Structure with Complete Implementation Details**

```
microgateway/
├── cmd/
│   └── microgateway/
│       └── main.go                 # Entry point
├── internal/
│   ├── config/
│   │   ├── config.go              # Main configuration struct
│   │   ├── env.go                 # Environment variable loader
│   │   └── validation.go          # Config validation
│   ├── database/
│   │   ├── migrations/
│   │   │   ├── 001_initial.up.sql
│   │   │   ├── 001_initial.down.sql
│   │   │   └── migrate.go
│   │   ├── connection.go          # Database connection manager
│   │   ├── models.go              # Extended GORM models
│   │   └── repository.go          # Repository pattern implementation
│   ├── services/
│   │   ├── gateway_service.go     # Implements ServiceInterface
│   │   ├── budget_service.go      # Implements BudgetServiceInterface
│   │   ├── analytics_service.go   # Analytics handler
│   │   ├── token_service.go       # Token management
│   │   └── management_service.go  # Management API business logic
│   ├── auth/
│   │   ├── interface.go           # AuthProvider interface
│   │   ├── token_auth.go          # Token authentication implementation
│   │   ├── cache.go               # Thread-safe token cache
│   │   └── middleware.go          # Auth middleware
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── llm_handlers.go    # LLM CRUD handlers
│   │   │   ├── app_handlers.go    # App CRUD handlers
│   │   │   ├── credential_handlers.go
│   │   │   ├── budget_handlers.go
│   │   │   ├── token_handlers.go
│   │   │   ├── health_handlers.go
│   │   │   └── analytics_handlers.go
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── logging.go
│   │   │   ├── cors.go
│   │   │   └── ratelimit.go
│   │   ├── validators/
│   │   │   └── request_validators.go
│   │   ├── responses/
│   │   │   └── standard_responses.go
│   │   └── router.go
│   └── server/
│       ├── server.go              # HTTP server implementation
│       ├── graceful.go            # Graceful shutdown
│       └── tls.go                 # TLS configuration
├── configs/
│   ├── .env.example
│   └── config.yaml.example
├── deployments/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── k8s/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       └── secret.yaml
├── scripts/
│   ├── setup.sh
│   ├── migrate.sh
│   └── test.sh
├── tests/
│   ├── integration/
│   └── e2e/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## **Phase 2: Database Schema and Models**

### **2.1 Complete Database Schema (PostgreSQL/SQLite Compatible)**

```sql
-- migrations/001_initial.up.sql

-- API Tokens table for gateway authentication
CREATE TABLE api_tokens (
    id SERIAL PRIMARY KEY,
    token VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    app_id INTEGER NOT NULL,
    scopes TEXT, -- JSON array of scopes
    is_active BOOLEAN DEFAULT true,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    INDEX idx_token_active (token, is_active),
    INDEX idx_app_tokens (app_id, is_active)
);

-- Token cache table for persistent cache backing
CREATE TABLE token_cache (
    token VARCHAR(255) PRIMARY KEY,
    cache_data JSONB NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_cache_expiry (expires_at)
);

-- Extended LLMs table
CREATE TABLE llms (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    vendor VARCHAR(100) NOT NULL,
    endpoint VARCHAR(500),
    api_key_encrypted TEXT, -- Encrypted API key
    default_model VARCHAR(255),
    max_tokens INTEGER DEFAULT 4096,
    timeout_seconds INTEGER DEFAULT 30,
    retry_count INTEGER DEFAULT 3,
    is_active BOOLEAN DEFAULT true,
    monthly_budget DECIMAL(10,2),
    rate_limit_rpm INTEGER, -- Requests per minute
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    INDEX idx_llm_active (is_active, slug),
    INDEX idx_llm_vendor (vendor, is_active)
);

-- Apps table
CREATE TABLE apps (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_email VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    monthly_budget DECIMAL(10,2),
    budget_start_date DATE,
    budget_reset_day INTEGER DEFAULT 1, -- Day of month to reset
    rate_limit_rpm INTEGER,
    allowed_ips TEXT, -- JSON array of allowed IPs
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    INDEX idx_app_active (is_active),
    INDEX idx_app_owner (owner_email)
);

-- Credentials table
CREATE TABLE credentials (
    id SERIAL PRIMARY KEY,
    app_id INTEGER NOT NULL,
    key_id VARCHAR(255) UNIQUE NOT NULL,
    secret_hash VARCHAR(255) NOT NULL, -- Hashed secret
    name VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    INDEX idx_cred_app (app_id, is_active),
    INDEX idx_cred_secret (secret_hash, is_active)
);

-- App-LLM associations
CREATE TABLE app_llms (
    app_id INTEGER NOT NULL,
    llm_id INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT true,
    custom_budget DECIMAL(10,2), -- Override app budget for specific LLM
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (app_id, llm_id),
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE CASCADE
);

-- Model pricing table
CREATE TABLE model_prices (
    id SERIAL PRIMARY KEY,
    vendor VARCHAR(100) NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    prompt_price DECIMAL(10,8) NOT NULL, -- Price per token
    completion_price DECIMAL(10,8) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    per_tokens INTEGER DEFAULT 1000, -- Price per X tokens
    effective_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_model_price (vendor, model_name, effective_date),
    INDEX idx_price_lookup (vendor, model_name)
);

-- Budget tracking table
CREATE TABLE budget_usage (
    id SERIAL PRIMARY KEY,
    app_id INTEGER NOT NULL,
    llm_id INTEGER,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    tokens_used BIGINT DEFAULT 0,
    requests_count INTEGER DEFAULT 0,
    total_cost DECIMAL(10,4) DEFAULT 0,
    prompt_tokens BIGINT DEFAULT 0,
    completion_tokens BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE SET NULL,
    UNIQUE KEY idx_budget_period (app_id, llm_id, period_start, period_end),
    INDEX idx_budget_app (app_id, period_start)
);

-- Analytics events table
CREATE TABLE analytics_events (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(100) UNIQUE NOT NULL,
    app_id INTEGER NOT NULL,
    llm_id INTEGER,
    credential_id INTEGER,
    endpoint VARCHAR(500),
    method VARCHAR(10),
    status_code INTEGER,
    request_tokens INTEGER,
    response_tokens INTEGER,
    total_tokens INTEGER,
    cost DECIMAL(10,6),
    latency_ms INTEGER,
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE,
    FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE SET NULL,
    INDEX idx_analytics_app (app_id, created_at),
    INDEX idx_analytics_request (request_id)
);

-- Filters table
CREATE TABLE filters (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'request', 'response', 'both'
    script TEXT NOT NULL, -- Filter script (Tengo or similar)
    is_active BOOLEAN DEFAULT true,
    order_index INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    INDEX idx_filter_active (is_active, order_index)
);

-- LLM-Filter associations
CREATE TABLE llm_filters (
    llm_id INTEGER NOT NULL,
    filter_id INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT true,
    order_index INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (llm_id, filter_id),
    FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE CASCADE,
    FOREIGN KEY (filter_id) REFERENCES filters(id) ON DELETE CASCADE
);
```

### **2.2 GORM Model Definitions**

```go
// internal/database/models.go
package database

import (
    "time"
    "gorm.io/gorm"
    "gorm.io/datatypes"
)

// APIToken represents an API token for gateway access
type APIToken struct {
    ID          uint           `gorm:"primaryKey"`
    Token       string         `gorm:"uniqueIndex;not null"`
    Name        string         `gorm:"not null"`
    AppID       uint           `gorm:"not null"`
    App         *App           `gorm:"foreignKey:AppID"`
    Scopes      datatypes.JSON `gorm:"type:json"`
    IsActive    bool           `gorm:"default:true;index:idx_token_active"`
    ExpiresAt   *time.Time
    LastUsedAt  *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TokenCache for persistent cache backing
type TokenCache struct {
    Token      string         `gorm:"primaryKey"`
    CacheData  datatypes.JSON `gorm:"not null;type:json"`
    ExpiresAt  time.Time      `gorm:"not null;index"`
    CreatedAt  time.Time
}

// Extended App model with additional fields
type App struct {
    gorm.Model
    Name            string         `gorm:"not null"`
    Description     string
    OwnerEmail      string         `gorm:"index"`
    IsActive        bool           `gorm:"default:true;index"`
    MonthlyBudget   float64
    BudgetStartDate *time.Time
    BudgetResetDay  int            `gorm:"default:1"`
    RateLimitRPM    int
    AllowedIPs      datatypes.JSON `gorm:"type:json"`
    Metadata        datatypes.JSON `gorm:"type:json"`
    
    // Relationships
    Credentials []Credential   `gorm:"foreignKey:AppID"`
    Tokens      []APIToken     `gorm:"foreignKey:AppID"`
    LLMs        []LLM          `gorm:"many2many:app_llms;"`
    BudgetUsage []BudgetUsage  `gorm:"foreignKey:AppID"`
}

// BudgetUsage tracks budget consumption
type BudgetUsage struct {
    ID               uint      `gorm:"primaryKey"`
    AppID            uint      `gorm:"not null;uniqueIndex:idx_budget_period"`
    App              *App      `gorm:"foreignKey:AppID"`
    LLMID            *uint     `gorm:"uniqueIndex:idx_budget_period"`
    LLM              *LLM      `gorm:"foreignKey:LLMID"`
    PeriodStart      time.Time `gorm:"not null;uniqueIndex:idx_budget_period"`
    PeriodEnd        time.Time `gorm:"not null;uniqueIndex:idx_budget_period"`
    TokensUsed       int64     `gorm:"default:0"`
    RequestsCount    int       `gorm:"default:0"`
    TotalCost        float64   `gorm:"default:0"`
    PromptTokens     int64     `gorm:"default:0"`
    CompletionTokens int64     `gorm:"default:0"`
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

// AnalyticsEvent for request/response tracking
type AnalyticsEvent struct {
    ID            uint           `gorm:"primaryKey"`
    RequestID     string         `gorm:"uniqueIndex;not null"`
    AppID         uint           `gorm:"not null;index:idx_analytics_app"`
    App           *App           `gorm:"foreignKey:AppID"`
    LLMID         *uint
    LLM           *LLM           `gorm:"foreignKey:LLMID"`
    CredentialID  *uint
    Credential    *Credential    `gorm:"foreignKey:CredentialID"`
    Endpoint      string
    Method        string
    StatusCode    int
    RequestTokens int
    ResponseTokens int
    TotalTokens   int
    Cost          float64
    LatencyMs     int
    ErrorMessage  string
    Metadata      datatypes.JSON `gorm:"type:json"`
    CreatedAt     time.Time      `gorm:"index:idx_analytics_app"`
}
```

## **Phase 3: Service Implementations**

### **3.1 Gateway Service Implementation**

```go
// internal/services/gateway_service.go
package services

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/TykTechnologies/midsommar/v2/models"
    "github.com/TykTechnologies/midsommar/v2/services"
    "github.com/TykTechnologies/midsommar/microgateway/internal/auth"
    "github.com/TykTechnologies/midsommar/microgateway/internal/database"
    "gorm.io/gorm"
)

type DatabaseGatewayService struct {
    db    *gorm.DB
    cache *auth.TokenCache
    mu    sync.RWMutex
}

func NewDatabaseGatewayService(db *gorm.DB, cache *auth.TokenCache) *DatabaseGatewayService {
    return &DatabaseGatewayService{
        db:    db,
        cache: cache,
    }
}

// GetActiveLLMs returns all active LLMs from database
func (s *DatabaseGatewayService) GetActiveLLMs() ([]models.LLM, error) {
    var llms []database.LLM
    err := s.db.Where("is_active = ? AND deleted_at IS NULL", true).
        Preload("Filters").
        Find(&llms).Error
    if err != nil {
        return nil, fmt.Errorf("failed to get active LLMs: %w", err)
    }
    
    // Convert to models.LLM
    result := make([]models.LLM, len(llms))
    for i, llm := range llms {
        result[i] = s.convertToModelLLM(llm)
    }
    return result, nil
}

// GetCredentialBySecret validates and returns credential
func (s *DatabaseGatewayService) GetCredentialBySecret(secret string) (*models.Credential, error) {
    // Check cache first
    if cached := s.cache.GetCredential(secret); cached != nil {
        return cached, nil
    }
    
    var cred database.Credential
    secretHash := hashSecret(secret)
    err := s.db.Where("secret_hash = ? AND is_active = ? AND deleted_at IS NULL", 
        secretHash, true).
        Preload("App").
        First(&cred).Error
        
    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, fmt.Errorf("invalid credentials")
        }
        return nil, fmt.Errorf("credential lookup failed: %w", err)
    }
    
    // Check expiration
    if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
        return nil, fmt.Errorf("credential expired")
    }
    
    // Update last used
    s.db.Model(&cred).Update("last_used_at", time.Now())
    
    modelCred := s.convertToModelCredential(cred)
    
    // Cache the credential
    s.cache.SetCredential(secret, modelCred, 5*time.Minute)
    
    return modelCred, nil
}

// GetAppByCredentialID returns app with all relationships
func (s *DatabaseGatewayService) GetAppByCredentialID(credID uint) (*models.App, error) {
    var app database.App
    err := s.db.Preload("LLMs").
        Preload("Credentials").
        Joins("JOIN credentials ON credentials.app_id = apps.id").
        Where("credentials.id = ? AND apps.is_active = ?", credID, true).
        First(&app).Error
        
    if err != nil {
        return nil, fmt.Errorf("app lookup failed: %w", err)
    }
    
    return s.convertToModelApp(app), nil
}

// Additional method implementations...
```

### **3.2 Token Authentication Service**

```go
// internal/auth/token_auth.go
package auth

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "sync"
    "time"
    
    "github.com/TykTechnologies/midsommar/microgateway/internal/database"
    "gorm.io/gorm"
)

type TokenAuthProvider struct {
    db    *gorm.DB
    cache *TokenCache
}

func NewTokenAuthProvider(db *gorm.DB) *TokenAuthProvider {
    return &TokenAuthProvider{
        db:    db,
        cache: NewTokenCache(1000, 1*time.Hour),
    }
}

// ValidateToken checks if a token is valid
func (p *TokenAuthProvider) ValidateToken(token string) (*AuthResult, error) {
    // Check cache first
    if cached := p.cache.Get(token); cached != nil {
        return &AuthResult{
            Valid:     true,
            AppID:     cached.AppID,
            Scopes:    cached.Scopes,
            ExpiresAt: cached.ExpiresAt,
        }, nil
    }
    
    // Query database
    var apiToken database.APIToken
    err := p.db.Where("token = ? AND is_active = ?", token, true).
        Preload("App").
        First(&apiToken).Error
        
    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return &AuthResult{Valid: false}, nil
        }
        return nil, fmt.Errorf("token validation failed: %w", err)
    }
    
    // Check expiration
    if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
        return &AuthResult{Valid: false}, nil
    }
    
    // Update last used
    p.db.Model(&apiToken).Update("last_used_at", time.Now())
    
    // Parse scopes
    var scopes []string
    if err := json.Unmarshal(apiToken.Scopes, &scopes); err != nil {
        scopes = []string{}
    }
    
    result := &AuthResult{
        Valid:     true,
        AppID:     apiToken.AppID,
        Scopes:    scopes,
        ExpiresAt: apiToken.ExpiresAt,
    }
    
    // Cache the result
    p.cache.Set(token, &CachedToken{
        Token:     token,
        AppID:     apiToken.AppID,
        Scopes:    scopes,
        ExpiresAt: apiToken.ExpiresAt,
        CreatedAt: time.Now(),
    })
    
    return result, nil
}

// GenerateToken creates a new API token
func (p *TokenAuthProvider) GenerateToken(appID uint, name string, scopes []string, expiresIn time.Duration) (string, error) {
    // Generate secure random token
    tokenBytes := make([]byte, 32)
    if _, err := rand.Read(tokenBytes); err != nil {
        return "", fmt.Errorf("failed to generate token: %w", err)
    }
    
    token := hex.EncodeToString(tokenBytes)
    
    // Calculate expiration
    var expiresAt *time.Time
    if expiresIn > 0 {
        exp := time.Now().Add(expiresIn)
        expiresAt = &exp
    }
    
    // Store in database
    scopesJSON, _ := json.Marshal(scopes)
    apiToken := database.APIToken{
        Token:     token,
        Name:      name,
        AppID:     appID,
        Scopes:    scopesJSON,
        IsActive:  true,
        ExpiresAt: expiresAt,
    }
    
    if err := p.db.Create(&apiToken).Error; err != nil {
        return "", fmt.Errorf("failed to store token: %w", err)
    }
    
    return token, nil
}
```

### **3.3 Thread-Safe Token Cache**

```go
// internal/auth/cache.go
package auth

import (
    "sync"
    "time"
)

type TokenCache struct {
    mu       sync.RWMutex
    tokens   map[string]*CachedToken
    creds    map[string]*CachedCredential
    maxSize  int
    ttl      time.Duration
    cleanupInterval time.Duration
    stopCleanup chan bool
}

type CachedToken struct {
    Token     string
    AppID     uint
    Scopes    []string
    ExpiresAt *time.Time
    CreatedAt time.Time
}

type CachedCredential struct {
    Credential *models.Credential
    CachedAt   time.Time
    TTL        time.Duration
}

func NewTokenCache(maxSize int, ttl time.Duration) *TokenCache {
    cache := &TokenCache{
        tokens:          make(map[string]*CachedToken),
        creds:           make(map[string]*CachedCredential),
        maxSize:         maxSize,
        ttl:             ttl,
        cleanupInterval: ttl / 2,
        stopCleanup:     make(chan bool),
    }
    
    // Start cleanup goroutine
    go cache.cleanupLoop()
    
    return cache
}

func (c *TokenCache) Get(token string) *CachedToken {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    cached, exists := c.tokens[token]
    if !exists {
        return nil
    }
    
    // Check if expired
    if time.Since(cached.CreatedAt) > c.ttl {
        return nil
    }
    
    if cached.ExpiresAt != nil && cached.ExpiresAt.Before(time.Now()) {
        return nil
    }
    
    return cached
}

func (c *TokenCache) Set(token string, cached *CachedToken) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Implement LRU if cache is full
    if len(c.tokens) >= c.maxSize {
        c.evictOldest()
    }
    
    c.tokens[token] = cached
}

func (c *TokenCache) cleanupLoop() {
    ticker := time.NewTicker(c.cleanupInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            c.cleanup()
        case <-c.stopCleanup:
            return
        }
    }
}

func (c *TokenCache) cleanup() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    now := time.Now()
    for token, cached := range c.tokens {
        if time.Since(cached.CreatedAt) > c.ttl ||
           (cached.ExpiresAt != nil && cached.ExpiresAt.Before(now)) {
            delete(c.tokens, token)
        }
    }
    
    for key, cached := range c.creds {
        if time.Since(cached.CachedAt) > cached.TTL {
            delete(c.creds, key)
        }
    }
}
```

## **Phase 4: Management API Implementation**

### **4.1 API Router Setup**

```go
// internal/api/router.go
package api

import (
    "github.com/gin-gonic/gin"
    "github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
    "github.com/TykTechnologies/midsommar/microgateway/internal/api/middleware"
    "github.com/TykTechnologies/midsommar/microgateway/internal/auth"
    "github.com/TykTechnologies/midsommar/microgateway/internal/services"
)

type RouterConfig struct {
    AuthProvider    auth.AuthProvider
    Services        *services.ServiceContainer
    EnableSwagger   bool
    EnableMetrics   bool
}

func SetupRouter(config *RouterConfig) *gin.Engine {
    router := gin.New()
    
    // Global middleware
    router.Use(gin.Recovery())
    router.Use(middleware.RequestLogger())
    router.Use(middleware.CORS())
    router.Use(middleware.RequestID())
    
    // Health endpoints (no auth)
    router.GET("/health", handlers.HealthCheck)
    router.GET("/ready", handlers.ReadinessCheck(config.Services))
    
    // API v1 routes
    v1 := router.Group("/api/v1")
    {
        // Public endpoints
        v1.POST("/auth/token", handlers.GenerateToken(config.Services))
        
        // Protected management endpoints
        protected := v1.Group("/")
        protected.Use(middleware.RequireAuth(config.AuthProvider))
        protected.Use(middleware.RequireScopes("admin"))
        
        // LLM management
        llms := protected.Group("/llms")
        {
            llms.GET("", handlers.ListLLMs(config.Services))
            llms.POST("", handlers.CreateLLM(config.Services))
            llms.GET("/:id", handlers.GetLLM(config.Services))
            llms.PUT("/:id", handlers.UpdateLLM(config.Services))
            llms.DELETE("/:id", handlers.DeleteLLM(config.Services))
            llms.GET("/:id/stats", handlers.GetLLMStats(config.Services))
        }
        
        // App management
        apps := protected.Group("/apps")
        {
            apps.GET("", handlers.ListApps(config.Services))
            apps.POST("", handlers.CreateApp(config.Services))
            apps.GET("/:id", handlers.GetApp(config.Services))
            apps.PUT("/:id", handlers.UpdateApp(config.Services))
            apps.DELETE("/:id", handlers.DeleteApp(config.Services))
            
            // Credentials sub-resource
            apps.GET("/:id/credentials", handlers.ListCredentials(config.Services))
            apps.POST("/:id/credentials", handlers.CreateCredential(config.Services))
            apps.DELETE("/:id/credentials/:credId", handlers.DeleteCredential(config.Services))
            
            // LLM associations
            apps.GET("/:id/llms", handlers.GetAppLLMs(config.Services))
            apps.PUT("/:id/llms", handlers.UpdateAppLLMs(config.Services))
        }
        
        // Budget management
        budgets := protected.Group("/budgets")
        {
            budgets.GET("", handlers.ListBudgets(config.Services))
            budgets.GET("/:appId/usage", handlers.GetBudgetUsage(config.Services))
            budgets.PUT("/:appId", handlers.UpdateBudget(config.Services))
            budgets.GET("/:appId/history", handlers.GetBudgetHistory(config.Services))
        }
        
        // Token management
        tokens := protected.Group("/tokens")
        {
            tokens.GET("", handlers.ListTokens(config.Services))
            tokens.POST("", handlers.CreateToken(config.Services))
            tokens.DELETE("/:token", handlers.RevokeToken(config.Services))
            tokens.GET("/:token", handlers.GetTokenInfo(config.Services))
        }
        
        // Analytics
        analytics := protected.Group("/analytics")
        {
            analytics.GET("/events", handlers.GetAnalyticsEvents(config.Services))            
            analytics.GET("/summary", handlers.GetAnalyticsSummary(config.Services))
            analytics.GET("/costs", handlers.GetCostAnalysis(config.Services))
        }
        
        // Filters management
        filters := protected.Group("/filters")
        {
            filters.GET("", handlers.ListFilters(config.Services))
            filters.POST("", handlers.CreateFilter(config.Services))
            filters.GET("/:id", handlers.GetFilter(config.Services))
            filters.PUT("/:id", handlers.UpdateFilter(config.Services))
            filters.DELETE("/:id", handlers.DeleteFilter(config.Services))
        }
        
        // Model pricing
        pricing := protected.Group("/pricing")
        {
            pricing.GET("", handlers.ListModelPrices(config.Services))
            pricing.POST("", handlers.SetModelPrice(config.Services))
            pricing.PUT("/:id", handlers.UpdateModelPrice(config.Services))
        }
    }
    
    // Gateway proxy endpoints (using AI Gateway library)
    gateway := router.Group("/")
    gateway.Use(middleware.RequireAuth(config.AuthProvider))
    gateway.Use(middleware.ValidateGatewayRequest())
    {
        // These routes will be handled by the AI Gateway library
        gateway.Any("/llm/rest/:llmSlug/*path", handlers.ProxyToGateway(config.Services))
        gateway.Any("/llm/stream/:llmSlug/*path", handlers.ProxyToGateway(config.Services))
        gateway.Any("/tools/:toolSlug/*path", handlers.ProxyToGateway(config.Services))
        gateway.Any("/datasource/:dsSlug/*path", handlers.ProxyToGateway(config.Services))
    }
    
    // Metrics endpoint if enabled
    if config.EnableMetrics {
        router.GET("/metrics", handlers.PrometheusMetrics())
    }
    
    // Swagger documentation if enabled
    if config.EnableSwagger {
        router.GET("/swagger/*any", handlers.SwaggerHandler())
    }
    
    return router
}
```

### **4.2 LLM Management Handlers**

```go
// internal/api/handlers/llm_handlers.go
package handlers

import (
    "net/http"
    "strconv"
    
    "github.com/gin-gonic/gin"
    "github.com/TykTechnologies/midsommar/microgateway/internal/api/responses"
    "github.com/TykTechnologies/midsommar/microgateway/internal/services"
    "github.com/gosimple/slug"
)

// ListLLMs returns paginated list of LLMs
func ListLLMs(services *services.ServiceContainer) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Parse query parameters
        page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
        limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
        vendor := c.Query("vendor")
        isActive := c.DefaultQuery("active", "true") == "true"
        
        // Get LLMs from service
        llms, total, err := services.Management.ListLLMs(page, limit, vendor, isActive)
        if err != nil {
            c.JSON(http.StatusInternalServerError, responses.Error(err.Error()))
            return
        }
        
        c.JSON(http.StatusOK, responses.Paginated(llms, page, limit, total))
    }
}

// CreateLLM creates a new LLM configuration
func CreateLLM(services *services.ServiceContainer) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req CreateLLMRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, responses.ValidationError(err))
            return
        }
        
        // Validate the request
        if err := req.Validate(); err != nil {
            c.JSON(http.StatusBadRequest, responses.ValidationError(err))
            return
        }
        
        // Generate slug from name
        llmSlug := slug.Make(req.Name)
        
        // Check if slug already exists
        if exists, _ := services.Management.LLMSlugExists(llmSlug); exists {
            c.JSON(http.StatusConflict, responses.Error("LLM with this name already exists"))
            return
        }
        
        // Encrypt API key if provided
        encryptedKey := ""
        if req.APIKey != "" {
            encryptedKey, _ = services.Crypto.Encrypt(req.APIKey)
        }
        
        // Create LLM
        llm := &database.LLM{
            Name:            req.Name,
            Slug:            llmSlug,
            Vendor:          req.Vendor,
            Endpoint:        req.Endpoint,
            APIKeyEncrypted: encryptedKey,
            DefaultModel:    req.DefaultModel,
            MaxTokens:       req.MaxTokens,
            TimeoutSeconds:  req.TimeoutSeconds,
            RetryCount:      req.RetryCount,
            IsActive:        req.IsActive,
            MonthlyBudget:   req.MonthlyBudget,
            RateLimitRPM:    req.RateLimitRPM,
            Metadata:        req.Metadata,
        }
        
        createdLLM, err := services.Management.CreateLLM(llm)
        if err != nil {
            c.JSON(http.StatusInternalServerError, responses.Error(err.Error()))
            return
        }
        
        // Reload gateway configuration
        services.Gateway.Reload()
        
        c.JSON(http.StatusCreated, responses.Success(createdLLM))
    }
}

// GetLLM retrieves a specific LLM by ID
func GetLLM(services *services.ServiceContainer) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, err := strconv.ParseUint(c.Param("id"), 10, 32)
        if err != nil {
            c.JSON(http.StatusBadRequest, responses.Error("Invalid LLM ID"))
            return
        }
        
        llm, err := services.Management.GetLLM(uint(id))
        if err != nil {
            c.JSON(http.StatusNotFound, responses.Error("LLM not found"))
            return
        }
        
        c.JSON(http.StatusOK, responses.Success(llm))
    }
}

// UpdateLLM updates an existing LLM
func UpdateLLM(services *services.ServiceContainer) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, err := strconv.ParseUint(c.Param("id"), 10, 32)
        if err != nil {
            c.JSON(http.StatusBadRequest, responses.Error("Invalid LLM ID"))
            return
        }
        
        var req UpdateLLMRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, responses.ValidationError(err))
            return
        }
        
        // Get existing LLM
        existingLLM, err := services.Management.GetLLM(uint(id))
        if err != nil {
            c.JSON(http.StatusNotFound, responses.Error("LLM not found"))
            return
        }
        
        // Update fields
        if req.Name != nil {
            existingLLM.Name = *req.Name
            existingLLM.Slug = slug.Make(*req.Name)
        }
        if req.Endpoint != nil {
            existingLLM.Endpoint = *req.Endpoint
        }
        if req.APIKey != nil && *req.APIKey != "" {
            encryptedKey, _ := services.Crypto.Encrypt(*req.APIKey)
            existingLLM.APIKeyEncrypted = encryptedKey
        }
        // ... update other fields
        
        updatedLLM, err := services.Management.UpdateLLM(existingLLM)
        if err != nil {
            c.JSON(http.StatusInternalServerError, responses.Error(err.Error()))
            return
        }
        
        // Reload gateway configuration
        services.Gateway.Reload()
        
        c.JSON(http.StatusOK, responses.Success(updatedLLM))
    }
}

// DeleteLLM soft deletes an LLM
func DeleteLLM(services *services.ServiceContainer) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, err := strconv.ParseUint(c.Param("id"), 10, 32)
        if err != nil {
            c.JSON(http.StatusBadRequest, responses.Error("Invalid LLM ID"))
            return
        }
        
        if err := services.Management.DeleteLLM(uint(id)); err != nil {
            c.JSON(http.StatusInternalServerError, responses.Error(err.Error()))
            return
        }
        
        // Reload gateway configuration
        services.Gateway.Reload()
        
        c.JSON(http.StatusOK, responses.Success(gin.H{"message": "LLM deleted successfully"}))
    }
}
```

### **4.3 Request/Response Models**

```go
// internal/api/handlers/models.go
package handlers

import (
    "encoding/json"
    "fmt"
    "time"
)

// CreateLLMRequest for creating new LLM
type CreateLLMRequest struct {
    Name           string          `json:"name" binding:"required,min=1,max=255"`
    Vendor         string          `json:"vendor" binding:"required,oneof=openai anthropic google vertex ollama"`
    Endpoint       string          `json:"endpoint"`
    APIKey         string          `json:"api_key"`
    DefaultModel   string          `json:"default_model" binding:"required"`
    MaxTokens      int             `json:"max_tokens,omitempty"`
    TimeoutSeconds int             `json:"timeout_seconds,omitempty"`
    RetryCount     int             `json:"retry_count,omitempty"`
    IsActive       bool            `json:"is_active"`
    MonthlyBudget  float64         `json:"monthly_budget,omitempty"`
    RateLimitRPM   int             `json:"rate_limit_rpm,omitempty"`
    Metadata       json.RawMessage `json:"metadata,omitempty"`
}

func (r *CreateLLMRequest) Validate() error {
    if r.MaxTokens == 0 {
        r.MaxTokens = 4096
    }
    if r.TimeoutSeconds == 0 {
        r.TimeoutSeconds = 30
    }
    if r.RetryCount == 0 {
        r.RetryCount = 3
    }
    
    // Validate vendor-specific requirements
    switch r.Vendor {
    case "openai":
        if r.APIKey == "" {
            return fmt.Errorf("API key is required for OpenAI")
        }
    case "ollama":
        if r.Endpoint == "" {
            return fmt.Errorf("endpoint is required for Ollama")
        }
    }
    
    return nil
}

// CreateAppRequest for creating new app
type CreateAppRequest struct {
    Name           string   `json:"name" binding:"required,min=1,max=255"`
    Description    string   `json:"description"`
    OwnerEmail     string   `json:"owner_email" binding:"required,email"`
    MonthlyBudget  float64  `json:"monthly_budget"`
    BudgetResetDay int      `json:"budget_reset_day,omitempty"`
    RateLimitRPM   int      `json:"rate_limit_rpm,omitempty"`
    AllowedIPs     []string `json:"allowed_ips,omitempty"`
    LLMIDs         []uint   `json:"llm_ids"`
}

// CreateTokenRequest for generating API tokens
type CreateTokenRequest struct {
    AppID      uint          `json:"app_id" binding:"required"`
    Name       string        `json:"name" binding:"required"`
    Scopes     []string      `json:"scopes"`
    ExpiresIn  time.Duration `json:"expires_in,omitempty"` // Duration in seconds
}

// TokenResponse for token creation response
type TokenResponse struct {
    Token     string     `json:"token"`
    Name      string     `json:"name"`
    AppID     uint       `json:"app_id"`
    Scopes    []string   `json:"scopes"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}
```

## **Phase 5: Configuration Management**

### **5.1 Environment Configuration**

```go
// internal/config/config.go
package config

import (
    "fmt"
    "time"
    
    "github.com/caarlos0/env/v6"
    "github.com/joho/godotenv"
)

type Config struct {
    // Server Configuration
    Server ServerConfig
    
    // Database Configuration
    Database DatabaseConfig
    
    // Cache Configuration
    Cache CacheConfig
    
    // Gateway Configuration
    Gateway GatewayConfig
    
    // Analytics Configuration
    Analytics AnalyticsConfig
    
    // Security Configuration
    Security SecurityConfig
    
    // Observability Configuration
    Observability ObservabilityConfig
}

type ServerConfig struct {
    Port            int           `env:"PORT" envDefault:"8080"`
    Host            string        `env:"HOST" envDefault:"0.0.0.0"`
    TLSEnabled      bool          `env:"TLS_ENABLED" envDefault:"false"`
    TLSCertPath     string        `env:"TLS_CERT_PATH"`
    TLSKeyPath      string        `env:"TLS_KEY_PATH"`
    ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
    WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
    IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
    ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

type DatabaseConfig struct {
    Type            string        `env:"DATABASE_TYPE" envDefault:"sqlite"` // sqlite or postgres
    DSN             string        `env:"DATABASE_DSN" envDefault:"file:./data/microgateway.db?cache=shared&mode=rwc"`
    MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
    MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"25"`
    ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
    AutoMigrate     bool          `env:"DB_AUTO_MIGRATE" envDefault:"true"`
    LogLevel        string        `env:"DB_LOG_LEVEL" envDefault:"warn"`
}

type CacheConfig struct {
    Enabled          bool          `env:"CACHE_ENABLED" envDefault:"true"`
    MaxSize          int           `env:"CACHE_MAX_SIZE" envDefault:"1000"`
    TTL              time.Duration `env:"CACHE_TTL" envDefault:"1h"`
    CleanupInterval  time.Duration `env:"CACHE_CLEANUP_INTERVAL" envDefault:"10m"`
    PersistToDB      bool          `env:"CACHE_PERSIST_TO_DB" envDefault:"false"`
}

type GatewayConfig struct {
    Timeout         time.Duration `env:"GATEWAY_TIMEOUT" envDefault:"30s"`
    MaxRequestSize  int64         `env:"GATEWAY_MAX_REQUEST_SIZE" envDefault:"10485760"` // 10MB
    MaxResponseSize int64         `env:"GATEWAY_MAX_RESPONSE_SIZE" envDefault:"52428800"` // 50MB
    RateLimitRPM    int           `env:"GATEWAY_DEFAULT_RATE_LIMIT" envDefault:"100"`
    EnableFilters   bool          `env:"GATEWAY_ENABLE_FILTERS" envDefault:"true"`
    EnableAnalytics bool          `env:"GATEWAY_ENABLE_ANALYTICS" envDefault:"true"`
}

type AnalyticsConfig struct {
    Enabled         bool          `env:"ANALYTICS_ENABLED" envDefault:"true"`
    BufferSize      int           `env:"ANALYTICS_BUFFER_SIZE" envDefault:"1000"`
    FlushInterval   time.Duration `env:"ANALYTICS_FLUSH_INTERVAL" envDefault:"10s"`
    RetentionDays   int           `env:"ANALYTICS_RETENTION_DAYS" envDefault:"90"`
    EnableRealtime  bool          `env:"ANALYTICS_REALTIME" envDefault:"false"`
}

type SecurityConfig struct {
    JWTSecret          string        `env:"JWT_SECRET" envDefault:"change-me-in-production"`
    EncryptionKey      string        `env:"ENCRYPTION_KEY" envDefault:"change-me-in-production"`
    BCryptCost         int           `env:"BCRYPT_COST" envDefault:"10"`
    TokenLength        int           `env:"TOKEN_LENGTH" envDefault:"32"`
    SessionTimeout     time.Duration `env:"SESSION_TIMEOUT" envDefault:"24h"`
    EnableRateLimiting bool          `env:"ENABLE_RATE_LIMITING" envDefault:"true"`
    EnableIPWhitelist  bool          `env:"ENABLE_IP_WHITELIST" envDefault:"false"`
}

type ObservabilityConfig struct {
    LogLevel        string `env:"LOG_LEVEL" envDefault:"info"`
    LogFormat       string `env:"LOG_FORMAT" envDefault:"json"` // json or text
    EnableMetrics   bool   `env:"ENABLE_METRICS" envDefault:"true"`
    MetricsPath     string `env:"METRICS_PATH" envDefault:"/metrics"`
    EnableTracing   bool   `env:"ENABLE_TRACING" envDefault:"false"`
    TracingEndpoint string `env:"TRACING_ENDPOINT"`
    EnableProfiling bool   `env:"ENABLE_PROFILING" envDefault:"false"`
}

// Load reads configuration from environment and .env file
func Load(envFile string) (*Config, error) {
    // Load .env file if it exists
    if envFile != "" {
        if err := godotenv.Load(envFile); err != nil {
            // Not fatal if .env doesn't exist
            fmt.Printf("Warning: Could not load %s: %v\n", envFile, err)
        }
    }
    
    cfg := &Config{}
    if err := env.Parse(cfg); err != nil {
        return nil, fmt.Errorf("failed to parse environment variables: %w", err)
    }
    
    // Validate configuration
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid configuration: %w", err)
    }
    
    return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
    // Validate database configuration
    if c.Database.Type != "sqlite" && c.Database.Type != "postgres" {
        return fmt.Errorf("unsupported database type: %s", c.Database.Type)
    }
    
    // Validate TLS configuration
    if c.Server.TLSEnabled {
        if c.Server.TLSCertPath == "" || c.Server.TLSKeyPath == "" {
            return fmt.Errorf("TLS enabled but cert/key paths not provided")
        }
    }
    
    // Validate security keys
    if c.Security.JWTSecret == "change-me-in-production" {
        fmt.Println("Warning: Using default JWT secret. Change this in production!")
    }
    
    return nil
}
```

### **5.2 Environment File Template**

```bash
# configs/.env.example

# Server Configuration
PORT=8080
HOST=0.0.0.0
TLS_ENABLED=false
TLS_CERT_PATH=/path/to/cert.pem
TLS_KEY_PATH=/path/to/key.pem
READ_TIMEOUT=30s
WRITE_TIMEOUT=30s
SHUTDOWN_TIMEOUT=30s

# Database Configuration
# For PostgreSQL:
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://user:password@localhost:5432/microgateway?sslmode=disable
# For SQLite:
# DATABASE_TYPE=sqlite
# DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc

DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=5m
DB_AUTO_MIGRATE=true
DB_LOG_LEVEL=warn

# Cache Configuration
CACHE_ENABLED=true
CACHE_MAX_SIZE=1000
CACHE_TTL=1h
CACHE_CLEANUP_INTERVAL=10m
CACHE_PERSIST_TO_DB=false

# Gateway Configuration
GATEWAY_TIMEOUT=30s
GATEWAY_MAX_REQUEST_SIZE=10485760
GATEWAY_MAX_RESPONSE_SIZE=52428800
GATEWAY_DEFAULT_RATE_LIMIT=100
GATEWAY_ENABLE_FILTERS=true
GATEWAY_ENABLE_ANALYTICS=true

# Analytics Configuration
ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=1000
ANALYTICS_FLUSH_INTERVAL=10s
ANALYTICS_RETENTION_DAYS=90
ANALYTICS_REALTIME=false

# Security Configuration
JWT_SECRET=your-secret-key-here
ENCRYPTION_KEY=your-32-byte-encryption-key-here
BCRYPT_COST=10
TOKEN_LENGTH=32
SESSION_TIMEOUT=24h
ENABLE_RATE_LIMITING=true
ENABLE_IP_WHITELIST=false

# Observability Configuration
LOG_LEVEL=info
LOG_FORMAT=json
ENABLE_METRICS=true
METRICS_PATH=/metrics
ENABLE_TRACING=false
TRACING_ENDPOINT=http://localhost:14268/api/traces
ENABLE_PROFILING=false
```

## **Phase 6: Main Application Entry Point**

### **6.1 Main Application**

```go
// cmd/microgateway/main.go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/TykTechnologies/midsommar/microgateway/internal/config"
    "github.com/TykTechnologies/midsommar/microgateway/internal/database"
    "github.com/TykTechnologies/midsommar/microgateway/internal/server"
    "github.com/TykTechnologies/midsommar/microgateway/internal/services"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

func main() {
    // Parse command line flags
    var (
        envFile    = flag.String("env", ".env", "Path to environment file")
        migrate    = flag.Bool("migrate", false, "Run database migrations and exit")
        version    = flag.Bool("version", false, "Show version and exit")
        configFile = flag.String("config", "", "Path to config file (optional)")
    )
    flag.Parse()
    
    // Show version if requested
    if *version {
        fmt.Printf("Microgateway v%s (build: %s)\n", Version, BuildHash)
        os.Exit(0)
    }
    
    // Load configuration
    cfg, err := config.Load(*envFile)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load configuration")
    }
    
    // Setup logging
    setupLogging(cfg.Observability)
    
    log.Info().
        Str("version", Version).
        Str("build", BuildHash).
        Msg("Starting Microgateway")
    
    // Connect to database
    db, err := database.Connect(cfg.Database)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to connect to database")
    }
    defer func() {
        if sqlDB, err := db.DB(); err == nil {
            sqlDB.Close()
        }
    }()
    
    // Run migrations if requested
    if *migrate || cfg.Database.AutoMigrate {
        log.Info().Msg("Running database migrations...")
        if err := database.Migrate(db); err != nil {
            log.Fatal().Err(err).Msg("Failed to run migrations")
        }
        if *migrate {
            log.Info().Msg("Migrations completed successfully")
            os.Exit(0)
        }
    }
    
    // Initialize service container
    serviceContainer, err := services.NewServiceContainer(db, cfg)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to initialize services")
    }
    
    // Create and configure server
    srv, err := server.New(cfg, serviceContainer)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to create server")
    }
    
    // Setup signal handling for graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), 
        os.Interrupt, 
        syscall.SIGTERM,
    )
    defer stop()
    
    // Start server in goroutine
    serverErrors := make(chan error, 1)
    go func() {
        log.Info().
            Int("port", cfg.Server.Port).
            Bool("tls", cfg.Server.TLSEnabled).
            Msg("Starting HTTP server")
            
        if err := srv.Start(); err != nil {
            serverErrors <- err
        }
    }()
    
    // Start background tasks
    go serviceContainer.StartBackgroundTasks(ctx)
    
    // Wait for shutdown signal or server error
    select {
    case err := <-serverErrors:
        log.Error().Err(err).Msg("Server error")
        stop()
    case <-ctx.Done():
        log.Info().Msg("Shutdown signal received")
    }
    
    // Graceful shutdown
    log.Info().Msg("Starting graceful shutdown...")
    
    shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
    defer cancel()
    
    // Stop background tasks
    serviceContainer.StopBackgroundTasks()
    
    // Shutdown server
    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Error().Err(err).Msg("Server shutdown error")
    }
    
    // Final cleanup
    serviceContainer.Cleanup()
    
    log.Info().Msg("Microgateway stopped gracefully")
}

func setupLogging(cfg config.ObservabilityConfig) {
    // Set log level
    level, err := zerolog.ParseLevel(cfg.LogLevel)
    if err != nil {
        level = zerolog.InfoLevel
    }
    zerolog.SetGlobalLevel(level)
    
    // Configure output format
    if cfg.LogFormat == "text" {
        log.Logger = log.Output(zerolog.ConsoleWriter{
            Out:        os.Stdout,
            TimeFormat: time.RFC3339,
        })
    } else {
        log.Logger = log.With().Timestamp().Logger()
    }
}
```

### **6.2 Server Implementation**

```go
// internal/server/server.go
package server

import (
    "context"
    "fmt"
    "net/http"
    "time"
    
    "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
    "github.com/TykTechnologies/midsommar/microgateway/internal/api"
    "github.com/TykTechnologies/midsommar/microgateway/internal/config"
    "github.com/TykTechnologies/midsommar/microgateway/internal/services"
    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"
)

type Server struct {
    config   *config.Config
    services *services.ServiceContainer
    gateway  *aigateway.Gateway
    router   *gin.Engine
    server   *http.Server
}

func New(cfg *config.Config, services *services.ServiceContainer) (*Server, error) {
    // Initialize AI Gateway
    gatewayConfig := &aigateway.Config{
        Port:            cfg.Server.Port,
        Timeout:         cfg.Gateway.Timeout,
        MaxRequestSize:  cfg.Gateway.MaxRequestSize,
        MaxResponseSize: cfg.Gateway.MaxResponseSize,
    }
    
    gateway := aigateway.NewWithAnalytics(
        services.GatewayService,
        services.BudgetService,
        services.AnalyticsService,
        gatewayConfig,
    )
    
    // Setup API router
    routerConfig := &api.RouterConfig{
        AuthProvider:  services.AuthProvider,
        Services:      services,
        EnableSwagger: cfg.Observability.EnableMetrics,
        EnableMetrics: cfg.Observability.EnableMetrics,
    }
    
    router := api.SetupRouter(routerConfig)
    
    // Mount gateway handler
    router.Any("/llm/*path", gin.WrapH(gateway.Handler()))
    router.Any("/tools/*path", gin.WrapH(gateway.Handler()))
    router.Any("/datasource/*path", gin.WrapH(gateway.Handler()))
    
    // Create HTTP server
    server := &http.Server{
        Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
        Handler:      router,
        ReadTimeout:  cfg.Server.ReadTimeout,
        WriteTimeout: cfg.Server.WriteTimeout,
        IdleTimeout:  cfg.Server.IdleTimeout,
    }
    
    return &Server{
        config:   cfg,
        services: services,
        gateway:  gateway,
        router:   router,
        server:   server,
    }, nil
}

func (s *Server) Start() error {
    if s.config.Server.TLSEnabled {
        log.Info().Msg("Starting server with TLS")
        return s.server.ListenAndServeTLS(
            s.config.Server.TLSCertPath,
            s.config.Server.TLSKeyPath,
        )
    }
    
    log.Info().Msg("Starting server without TLS")
    return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    log.Info().Msg("Shutting down server...")
    
    // Stop accepting new connections
    if err := s.server.Shutdown(ctx); err != nil {
        return fmt.Errorf("server shutdown failed: %w", err)
    }
    
    // Stop gateway
    if err := s.gateway.Stop(ctx); err != nil {
        return fmt.Errorf("gateway shutdown failed: %w", err)
    }
    
    return nil
}
```

## **Phase 7: Service Container**

### **7.1 Service Container Implementation**

```go
// internal/services/container.go
package services

import (
    "context"
    "time"
    
    "github.com/TykTechnologies/midsommar/microgateway/internal/auth"
    "github.com/TykTechnologies/midsommar/microgateway/internal/config"
    "gorm.io/gorm"
)

type ServiceContainer struct {
    // Core services
    GatewayService   *DatabaseGatewayService
    BudgetService    *DatabaseBudgetService
    AnalyticsService *DatabaseAnalyticsService

    // Management services
    Management       *ManagementService
    TokenService     *TokenService
    
    // Authentication
    AuthProvider     auth.AuthProvider
    
    // Utilities
    Crypto           *CryptoService
    Cache            *auth.TokenCache
    
    // Background tasks
    analyticsBuffer  *AnalyticsBuffer
    budgetMonitor    *BudgetMonitor
    cacheCleanup     *CacheCleanupTask
    
    // Context for background tasks
    ctx              context.Context
    cancel           context.CancelFunc
}

func NewServiceContainer(db *gorm.DB, cfg *config.Config) (*ServiceContainer, error) {
    // Initialize cache
    cache := auth.NewTokenCache(cfg.Cache.MaxSize, cfg.Cache.TTL)
    
    // Initialize crypto service
    crypto := NewCryptoService(cfg.Security.EncryptionKey)
    
    // Initialize auth provider
    authProvider := auth.NewTokenAuthProvider(db, cache)
    
    // Initialize core services
    gatewayService := NewDatabaseGatewayService(db, cache)
    budgetService := NewDatabaseBudgetService(db)
    analyticsService := NewDatabaseAnalyticsService(db, cfg.Analytics)
    
    // Initialize management services
    management := NewManagementService(db, crypto)
    tokenService := NewTokenService(db, authProvider)
    
    // Initialize background tasks
    analyticsBuffer := NewAnalyticsBuffer(analyticsService, cfg.Analytics.BufferSize)
    budgetMonitor := NewBudgetMonitor(budgetService, db)
    cacheCleanup := NewCacheCleanupTask(cache, db)
    
    ctx, cancel := context.WithCancel(context.Background())
    
    return &ServiceContainer{
        GatewayService:   gatewayService,
        BudgetService:    budgetService,
        AnalyticsService: analyticsService,
        Management:       management,
        TokenService:     tokenService,
        AuthProvider:     authProvider,
        Crypto:           crypto,
        Cache:            cache,
        analyticsBuffer:  analyticsBuffer,
        budgetMonitor:    budgetMonitor,
        cacheCleanup:     cacheCleanup,
        ctx:              ctx,
        cancel:           cancel,
    }, nil
}

func (sc *ServiceContainer) StartBackgroundTasks(ctx context.Context) {
    // Start analytics buffer flush
    go sc.analyticsBuffer.Start(ctx)
    
    // Start budget monitoring
    go sc.budgetMonitor.Start(ctx)
    
    // Start cache cleanup
    go sc.cacheCleanup.Start(ctx)
    
    log.Info().Msg("Background tasks started")
}

func (sc *ServiceContainer) StopBackgroundTasks() {
    sc.cancel()
    
    // Wait for tasks to complete
    sc.analyticsBuffer.Stop()
    sc.budgetMonitor.Stop()
    sc.cacheCleanup.Stop()
    
    log.Info().Msg("Background tasks stopped")
}

func (sc *ServiceContainer) Cleanup() {
    // Flush any remaining analytics
    sc.analyticsBuffer.Flush()
    
    // Close cache
    sc.Cache.Close()
    
    log.Info().Msg("Service container cleanup completed")
}
```

### **7.2 Analytics Service with Buffering**

```go
// internal/services/analytics_service.go
package services

import (
    "context"
    "sync"
    "time"
    
    "github.com/TykTechnologies/midsommar/v2/analytics"
    "github.com/TykTechnologies/midsommar/microgateway/internal/config"
    "github.com/TykTechnologies/midsommar/microgateway/internal/database"
    "gorm.io/gorm"
    "github.com/rs/zerolog/log"
)

type DatabaseAnalyticsService struct {
    db       *gorm.DB
    config   config.AnalyticsConfig
    buffer   []database.AnalyticsEvent
    mu       sync.Mutex
    flushCh  chan bool
}

func NewDatabaseAnalyticsService(db *gorm.DB, cfg config.AnalyticsConfig) *DatabaseAnalyticsService {
    return &DatabaseAnalyticsService{
        db:      db,
        config:  cfg,
        buffer:  make([]database.AnalyticsEvent, 0, cfg.BufferSize),
        flushCh: make(chan bool, 1),
    }
}

// RecordRequest implements analytics.Handler interface
func (s *DatabaseAnalyticsService) RecordRequest(ctx context.Context, record *analytics.Record) error {
    event := database.AnalyticsEvent{
        RequestID:      record.RequestID,
        AppID:          record.APIID,
        LLMID:          &record.OrgID,
        Endpoint:       record.Path,
        Method:         record.Method,
        StatusCode:     record.ResponseCode,
        RequestTokens:  record.RequestTokens,
        ResponseTokens: record.ResponseTokens,
        TotalTokens:    record.TotalTokens,
        Cost:           record.Cost,
        LatencyMs:      int(record.RequestTime),
        Metadata:       record.Tags,
        CreatedAt:      time.Now(),
    }
    
    s.mu.Lock()
    s.buffer = append(s.buffer, event)
    shouldFlush := len(s.buffer) >= s.config.BufferSize
    s.mu.Unlock()
    
    if shouldFlush {
        select {
        case s.flushCh <- true:
        default:
            // Channel already has a flush request
        }
    }
    
    // Update budget usage in real-time if enabled
    if s.config.EnableRealtime {
        go s.updateBudgetUsage(event)
    }
    
    return nil
}

func (s *DatabaseAnalyticsService) Flush() error {
    s.mu.Lock()
    if len(s.buffer) == 0 {
        s.mu.Unlock()
        return nil
    }
    
    events := make([]database.AnalyticsEvent, len(s.buffer))
    copy(events, s.buffer)
    s.buffer = s.buffer[:0]
    s.mu.Unlock()
    
    // Batch insert
    if err := s.db.CreateInBatches(events, 100).Error; err != nil {
        log.Error().Err(err).Msg("Failed to flush analytics buffer")
        return err
    }
    
    log.Debug().Int("count", len(events)).Msg("Flushed analytics events")
    return nil
}

func (s *DatabaseAnalyticsService) updateBudgetUsage(event database.AnalyticsEvent) {
    // Update budget usage in real-time
    var usage database.BudgetUsage
    
    now := time.Now()
    periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
    periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
    
    // Find or create budget usage record
    err := s.db.Where(database.BudgetUsage{
        AppID:       event.AppID,
        LLMID:       event.LLMID,
        PeriodStart: periodStart,
        PeriodEnd:   periodEnd,
    }).FirstOrCreate(&usage).Error
    
    if err != nil {
        log.Error().Err(err).Msg("Failed to update budget usage")
        return
    }
    
    // Update usage
    usage.TokensUsed += int64(event.TotalTokens)
    usage.RequestsCount++
    usage.TotalCost += event.Cost
    usage.PromptTokens += int64(event.RequestTokens)
    usage.CompletionTokens += int64(event.ResponseTokens)
    
    if err := s.db.Save(&usage).Error; err != nil {
        log.Error().Err(err).Msg("Failed to save budget usage")
    }
}

// AnalyticsBuffer for background flushing
type AnalyticsBuffer struct {
    service *DatabaseAnalyticsService
    ticker  *time.Ticker
    done    chan bool
    wg      sync.WaitGroup
}

func NewAnalyticsBuffer(service *DatabaseAnalyticsService, bufferSize int) *AnalyticsBuffer {
    return &AnalyticsBuffer{
        service: service,
        done:    make(chan bool),
    }
}

func (ab *AnalyticsBuffer) Start(ctx context.Context) {
    ab.ticker = time.NewTicker(ab.service.config.FlushInterval)
    ab.wg.Add(1)
    
    defer ab.wg.Done()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ab.done:
            return
        case <-ab.ticker.C:
            ab.service.Flush()
        case <-ab.service.flushCh:
            ab.service.Flush()
        }
    }
}

func (ab *AnalyticsBuffer) Stop() {
    ab.ticker.Stop()
    close(ab.done)
    ab.wg.Wait()
}

func (ab *AnalyticsBuffer) Flush() {
    ab.service.Flush()
}
```

## **Phase 8: Deployment Configuration**

### **8.1 Dockerfile**

```dockerfile
# deployments/Dockerfile

# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always) -X main.BuildHash=$(git rev-parse HEAD)" \
    -o microgateway \
    ./cmd/microgateway

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -D -u 1000 -G gateway gateway

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/microgateway .

# Copy default configuration
COPY --from=builder /build/configs/.env.example .env.example

# Create data directory
RUN mkdir -p /app/data && \
    chown -R gateway:gateway /app

# Switch to non-root user
USER gateway

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
ENTRYPOINT ["./microgateway"]
```

### **8.2 Docker Compose for Development**

```yaml
# deployments/docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: microgateway
      POSTGRES_USER: gateway
      POSTGRES_PASSWORD: gateway123
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - gateway-net
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gateway"]
      interval: 10s
      timeout: 5s
      retries: 5

  microgateway:
    build:
      context: ..
      dockerfile: deployments/Dockerfile
    environment:
      DATABASE_TYPE: postgres
      DATABASE_DSN: postgres://gateway:gateway123@postgres:5432/microgateway?sslmode=disable
      JWT_SECRET: development-secret-key
      ENCRYPTION_KEY: development-encryption-key-32chars!
      LOG_LEVEL: debug
      DB_AUTO_MIGRATE: "true"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ./configs:/app/configs
      - gateway_data:/app/data
    networks:
      - gateway-net
    restart: unless-stopped

  # Optional: Redis for caching
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - gateway-net
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  gateway_data:
  redis_data:

networks:
  gateway-net:
    driver: bridge
```

### **8.3 Kubernetes Deployment**

```yaml
# deployments/k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway
  namespace: ai-gateway
  labels:
    app: microgateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: microgateway
  template:
    metadata:
      labels:
        app: microgateway
    spec:
      containers:
      - name: microgateway
        image: microgateway:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: DATABASE_TYPE
          value: "postgres"
        - name: DATABASE_DSN
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: database-dsn
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: jwt-secret
        - name: ENCRYPTION_KEY
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: encryption-key
        envFrom:
        - configMapRef:
            name: microgateway-config
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /health
            port: http
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: config
          mountPath: /app/configs
      volumes:
      - name: config
        configMap:
          name: microgateway-config
---
apiVersion: v1
kind: Service
metadata:
  name: microgateway
  namespace: ai-gateway
spec:
  selector:
    app: microgateway
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
  type: LoadBalancer
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: microgateway-config
  namespace: ai-gateway
data:
  LOG_LEVEL: "info"
  CACHE_ENABLED: "true"
  ANALYTICS_ENABLED: "true"
  GATEWAY_TIMEOUT: "30s"
---
apiVersion: v1
kind: Secret
metadata:
  name: microgateway-secrets
  namespace: ai-gateway
type: Opaque
stringData:
  database-dsn: "postgres://user:password@postgres:5432/microgateway"
  jwt-secret: "your-production-jwt-secret"
  encryption-key: "your-32-byte-production-encryption-key"
```

## **Phase 9: Testing Strategy**

### **9.1 Unit Tests**

```go
// internal/services/gateway_service_test.go
package services

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    
    // Run migrations
    err = db.AutoMigrate(&database.App{}, &database.LLM{}, &database.Credential{})
    require.NoError(t, err)
    
    return db
}

func TestGatewayService_GetActiveLLMs(t *testing.T) {
    db := setupTestDB(t)
    cache := auth.NewTokenCache(100, 5*time.Minute)
    service := NewDatabaseGatewayService(db, cache)
    
    // Create test LLMs
    testLLMs := []database.LLM{
        {
            Name:     "Test GPT-4",
            Slug:     "test-gpt-4",
            Vendor:   "openai",
            IsActive: true,
        },
        {
            Name:     "Test Claude",
            Slug:     "test-claude",
            Vendor:   "anthropic",
            IsActive: false,
        },
    }
    
    for _, llm := range testLLMs {
        err := db.Create(&llm).Error
        require.NoError(t, err)
    }
    
    // Test GetActiveLLMs
    activeLLMs, err := service.GetActiveLLMs()
    assert.NoError(t, err)
    assert.Len(t, activeLLMs, 1)
    assert.Equal(t, "Test GPT-4", activeLLMs[0].Name)
}

func TestGatewayService_GetCredentialBySecret(t *testing.T) {
    db := setupTestDB(t)
    cache := auth.NewTokenCache(100, 5*time.Minute)
    service := NewDatabaseGatewayService(db, cache)
    
    // Create test app and credential
    app := database.App{
        Name:     "Test App",
        IsActive: true,
    }
    err := db.Create(&app).Error
    require.NoError(t, err)
    
    secret := "test-secret-123"
    cred := database.Credential{
        AppID:      app.ID,
        KeyID:      "test-key",
        SecretHash: hashSecret(secret),
        IsActive:   true,
    }
    err = db.Create(&cred).Error
    require.NoError(t, err)
    
    // Test valid credential
    result, err := service.GetCredentialBySecret(secret)
    assert.NoError(t, err)
    assert.NotNil(t, result)
    assert.Equal(t, "test-key", result.KeyID)
    
    // Test invalid credential
    result, err = service.GetCredentialBySecret("invalid-secret")
    assert.Error(t, err)
    assert.Nil(t, result)
    
    // Test cached credential
    result, err = service.GetCredentialBySecret(secret)
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

### **9.2 Integration Tests**

```go
// tests/integration/api_test.go
package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestLLMManagementAPI(t *testing.T) {
    // Setup test server
    router, services := setupTestServer(t)
    
    // Test Create LLM
    createReq := handlers.CreateLLMRequest{
        Name:         "Test LLM",
        Vendor:       "openai",
        DefaultModel: "gpt-4",
        IsActive:     true,
    }
    
    body, _ := json.Marshal(createReq)
    req := httptest.NewRequest("POST", "/api/v1/llms", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer test-admin-token")
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusCreated, w.Code)
    
    var response map[string]interface{}
    err := json.Unmarshal(w.Body.Bytes(), &response)
    require.NoError(t, err)
    assert.Contains(t, response, "data")
    
    // Test List LLMs
    req = httptest.NewRequest("GET", "/api/v1/llms", nil)
    req.Header.Set("Authorization", "Bearer test-admin-token")
    
    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
}

func TestGatewayProxy(t *testing.T) {
    // Setup test server with mock LLM backend
    router, services := setupTestServer(t)
    mockLLM := setupMockLLMServer()
    defer mockLLM.Close()
    
    // Create test app and credentials
    app, cred := createTestAppWithCredentials(t, services)
    
    // Test proxy request
    llmReq := map[string]interface{}{
        "model": "gpt-4",
        "messages": []map[string]string{
            {"role": "user", "content": "Hello"},
        },
    }
    
    body, _ := json.Marshal(llmReq)
    req := httptest.NewRequest("POST", "/llm/rest/test-llm/chat/completions", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer " + cred.Secret)
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
    
    // Verify analytics were recorded
    analytics, err := services.Analytics.GetEvents(app.ID, 1, 10)
    assert.NoError(t, err)
    assert.Len(t, analytics, 1)
}
```

### **9.3 End-to-End Tests**

```go
// tests/e2e/gateway_e2e_test.go
package e2e

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/suite"
)

type GatewayE2ETestSuite struct {
    suite.Suite
    serverURL string
    adminToken string
    appToken string
}

func (s *GatewayE2ETestSuite) SetupSuite() {
    // Start actual server
    s.serverURL = startTestServer()
    
    // Wait for server to be ready
    s.waitForServer()
    
    // Create admin token
    s.adminToken = s.createAdminToken()
}

func (s *GatewayE2ETestSuite) TestCompleteFlow() {
    // 1. Create LLM configuration
    llmID := s.createLLM("Test GPT-4", "openai")
    
    // 2. Create application
    appID := s.createApp("Test App")
    
    // 3. Associate LLM with app
    s.associateLLMWithApp(appID, llmID)
    
    // 4. Generate app token
    s.appToken = s.generateAppToken(appID)
    
    // 5. Make gateway request
    response := s.makeGatewayRequest("/llm/rest/test-gpt-4/chat/completions", s.appToken)
    s.Assert().Equal(200, response.StatusCode)
    
    // 6. Check analytics
    analytics := s.getAnalytics(appID)
    s.Assert().NotEmpty(analytics)
    
    // 7. Check budget usage
    usage := s.getBudgetUsage(appID)
    s.Assert().Greater(usage.TotalCost, 0.0)
}

func TestGatewayE2E(t *testing.T) {
    suite.Run(t, new(GatewayE2ETestSuite))
}
```

## **Phase 10: Documentation**

### **10.1 README.md**

```markdown
# Microgateway

A production-ready microgateway for AI/LLM API management built on the Midsommar AI Gateway library.

## Features

- 🚀 **High Performance**: Built with Go for low latency and high throughput
- 🔐 **Token Authentication**: Secure API access with token-based authentication
- 📊 **Analytics & Monitoring**: Real-time analytics and usage tracking
- 💰 **Budget Management**: Per-app and per-LLM budget controls
- 🔄 **Multi-LLM Support**: OpenAI, Anthropic, Google, Vertex AI, Ollama
- 🗄️ **Database Flexibility**: PostgreSQL for production, SQLite for development
- 🎯 **Management API**: Full CRUD operations for all entities
- 🏥 **Health Checks**: Kubernetes-ready health and readiness endpoints
- 📦 **Easy Deployment**: Docker, Kubernetes, and binary distributions

## Quick Start

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/TykTechnologies/midsommar.git
cd midsommar/microgateway

# Start with Docker Compose
docker-compose -f deployments/docker-compose.yml up

# The gateway will be available at http://localhost:8080
```

### Using Binary

```bash
# Download the latest release
wget https://github.com/TykTechnologies/midsommar/releases/latest/download/microgateway-linux-amd64

# Make it executable
chmod +x microgateway-linux-amd64

# Create configuration
cp configs/.env.example .env
# Edit .env with your settings

# Run migrations
./microgateway-linux-amd64 -migrate

# Start the gateway
./microgateway-linux-amd64
```

## Configuration

The gateway is configured through environment variables. See `.env.example` for all available options.

Key configuration areas:
- **Server**: Port, TLS, timeouts
- **Database**: PostgreSQL or SQLite connection
- **Cache**: In-memory caching settings
- **Security**: JWT secrets, encryption keys
- **Analytics**: Buffer size, flush intervals

## API Documentation

### Management API

The management API is available at `/api/v1` and requires admin authentication.

#### LLM Management
- `GET /api/v1/llms` - List LLMs
- `POST /api/v1/llms` - Create LLM
- `GET /api/v1/llms/{id}` - Get LLM
- `PUT /api/v1/llms/{id}` - Update LLM
- `DELETE /api/v1/llms/{id}` - Delete LLM

#### App Management
- `GET /api/v1/apps` - List apps
- `POST /api/v1/apps` - Create app
- `GET /api/v1/apps/{id}` - Get app
- `PUT /api/v1/apps/{id}` - Update app
- `DELETE /api/v1/apps/{id}` - Delete app

### Gateway API

The gateway proxies requests to configured LLMs:

```bash
# OpenAI-compatible endpoint
POST /llm/rest/{llm-slug}/chat/completions

# Streaming endpoint
POST /llm/stream/{llm-slug}/chat/completions
```

## Development

### Prerequisites

- Go 1.21+
- PostgreSQL 14+ or SQLite 3
- Make (optional)

### Building from Source

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Build binary
go build -o microgateway ./cmd/microgateway

# Run with hot reload (development)
air
```

### Running Tests

```bash
# Unit tests
make test-unit

# Integration tests
make test-integration

# E2E tests
make test-e2e

# All tests
make test
```

## Deployment

### Kubernetes

```bash
# Create namespace
kubectl create namespace ai-gateway

# Apply manifests
kubectl apply -f deployments/k8s/

# Check status
kubectl -n ai-gateway get pods
```

### Production Checklist

- [ ] Change default JWT secret
- [ ] Set strong encryption key
- [ ] Configure TLS certificates
- [ ] Set up database backups
- [ ] Configure monitoring/alerting
- [ ] Set appropriate resource limits
- [ ] Enable audit logging
- [ ] Configure rate limiting
- [ ] Set up log aggregation

### **10.2 Makefile**

```makefile
# microgateway/Makefile

# Variables
BINARY_NAME=microgateway
VERSION=$(shell git describe --tags --always --dirty)
BUILD_HASH=$(shell git rev-parse HEAD)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH}"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
CMD_DIR=./cmd/microgateway
DIST_DIR=./dist

.PHONY: all build clean test coverage lint help

all: test build ## Run tests and build

build: ## Build the binary
	$(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(CMD_DIR)

build-all: ## Build for all platforms
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(DIST_DIR)

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-unit: ## Run unit tests
	$(GOTEST) -v -short ./...

test-integration: ## Run integration tests
	$(GOTEST) -v -run Integration ./tests/integration

test-e2e: ## Run end-to-end tests
	$(GOTEST) -v -run E2E ./tests/e2e

coverage: test ## Generate coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run

deps: ## Download dependencies
	$(GOMOD) download

tidy: ## Tidy dependencies
	$(GOMOD) tidy

docker-build: ## Build Docker image
	docker build -f deployments/Dockerfile -t microgateway:$(VERSION) .

docker-push: ## Push Docker image
	docker tag microgateway:$(VERSION) your-registry/microgateway:$(VERSION)
	docker push your-registry/microgateway:$(VERSION)

docker-compose-up: ## Start with Docker Compose
	docker-compose -f deployments/docker-compose.yml up -d

docker-compose-down: ## Stop Docker Compose
	docker-compose -f deployments/docker-compose.yml down

migrate: ## Run database migrations
	$(GOBUILD) -o $(DIST_DIR)/$(BINARY_NAME) $(CMD_DIR)
	./$(DIST_DIR)/$(BINARY_NAME) -migrate

run: build ## Build and run
	./$(DIST_DIR)/$(BINARY_NAME)

dev: ## Run with hot reload (requires air)
	air

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
```

# Task

Above we havbe outlined a full and comprehensive plan for developing the microgateway, please begin developing the gateway.

Track your progress as you go so that if we need to stop and start over time we can pick up where we left off. To do so, make sure that you accurately document your milestones in a `milestones` folder in the project. Make sure to also check this folder to see if there is alrady work completed that you can continue from. If there is no folder, then create one.