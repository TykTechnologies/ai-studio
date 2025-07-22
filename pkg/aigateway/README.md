# AI Gateway Library

The AI Gateway library provides a simple interface to use the Midsommar AI Gateway as an importable library in standalone applications.

## Features

- **LLM Proxying**: Secure reverse proxy for multiple LLM providers (OpenAI, Anthropic, Google AI, etc.)
- **Authentication & Authorization**: Token-based authentication with app-level access control
- **Budget Enforcement**: Real-time budget tracking and limits
- **Request Filtering**: JavaScript-based policy enforcement
- **Analytics**: Usage tracking and cost calculation
- **Tool Integration**: REST API tool execution and MCP (Model Context Protocol) support
- **Data Source Proxying**: Vector search capabilities
- **Streaming Support**: Real-time streaming responses
- **Hot Reloading**: Dynamic configuration updates

## Installation

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
```

## Basic Usage

### Standalone Server

```go
package main

import (
    "log"
    
    "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
    "github.com/TykTechnologies/midsommar/v2/proxy"
    "github.com/TykTechnologies/midsommar/v2/services"
)

func main() {
    // Initialize your database and services
    db := setupDatabase()
    service := services.NewService(db)
    budgetService := services.NewBudgetService(db, service)

    // Create the gateway
    gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)

    // Start as standalone server
    log.Println("Starting AI Gateway on :9090")
    if err := gateway.Start(); err != nil {
        log.Fatal(err)
    }
}
```

### Integration with Existing HTTP Server

```go
package main

import (
    "log"
    "net/http"
    
    "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
    "github.com/TykTechnologies/midsommar/v2/proxy"
    "github.com/TykTechnologies/midsommar/v2/services"
)

func main() {
    // Initialize services
    db := setupDatabase()
    service := services.NewService(db)
    budgetService := services.NewBudgetService(db, service)
    
    // Create gateway (note: Port in config is ignored when using as handler)
    gateway := aigateway.New(service, &proxy.Config{}, budgetService)

    // Mount gateway in existing server
    mux := http.NewServeMux()
    mux.Handle("/ai/", http.StripPrefix("/ai", gateway.Handler()))
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })

    log.Println("Starting server on :8080 with AI Gateway at /ai/")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### Graceful Shutdown

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
    "github.com/TykTechnologies/midsommar/v2/proxy"
    "github.com/TykTechnologies/midsommar/v2/services"
)

func main() {
    // Initialize services
    db := setupDatabase()
    service := services.NewService(db)
    budgetService := services.NewBudgetService(db, service)
    gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)

    // Set up graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Start server in goroutine
    go func() {
        log.Println("Starting AI Gateway microproxy on :9090")
        if err := gateway.Start(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Gateway error: %v", err)
        }
    }()

    // Wait for shutdown signal
    <-ctx.Done()
    log.Println("Shutting down gateway...")

    // Graceful shutdown with timeout
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := gateway.Stop(shutdownCtx); err != nil {
        log.Printf("Gateway shutdown error: %v", err)
    }
    log.Println("Gateway stopped")
}
```

## Integration with Popular Frameworks

### Gin

```go
import "github.com/gin-gonic/gin"

router := gin.Default()
router.Any("/ai/*path", gin.WrapH(gateway.Handler()))
```

### Gorilla Mux

```go
import "github.com/gorilla/mux"

router := mux.NewRouter()
router.PathPrefix("/ai/").Handler(gateway.Handler())
```

### Chi

```go
import "github.com/go-chi/chi/v5"

router := chi.NewRouter()
router.Mount("/ai", gateway.Handler())
```

## API Interface

```go
type Gateway interface {
    // Start starts the gateway as an HTTP server on the configured port
    Start() error

    // Stop gracefully stops the gateway server  
    Stop(ctx context.Context) error

    // Handler returns an http.Handler for integration with existing servers
    Handler() http.Handler

    // Reload reloads the gateway configuration (LLMs, datasources, filters)
    Reload() error
}
```

## Configuration Hot Reloading

The gateway supports hot reloading of configuration without restarting:

```go
// After updating database configurations
if err := gateway.Reload(); err != nil {
    log.Printf("Failed to reload configuration: %v", err)
}
```

## Supported Endpoints

When using the gateway, the following endpoints are available:

### LLM Endpoints
- `POST /llm/rest/{llmSlug}/{path}` - REST API calls to LLMs
- `POST /llm/stream/{llmSlug}/{path}` - Streaming calls to LLMs

### Tool Endpoints  
- `GET|POST|PUT|DELETE /tools/{toolSlug}` - Tool operation calls
- `POST /tools/{toolSlug}/mcp` - MCP StreamableHTTP transport
- `GET /tools/{toolSlug}/mcp/sse` - MCP SSE transport
- `POST /tools/{toolSlug}/mcp/message` - MCP message endpoint

### Data Source Endpoints
- `POST /datasource/{dsSlug}` - Vector search queries

### Metadata Endpoints
- `GET /.well-known/oauth-protected-resource` - OAuth2 resource metadata

## Dependencies

The AI Gateway requires:

- **Database**: For storing LLM configurations, apps, users, and analytics
- **Services Layer**: Business logic and database operations (`services.Service`)
- **Budget Service**: Budget enforcement and tracking (`services.BudgetService`)

## Example Microproxy Service

Here's a complete example of a standalone microproxy service:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
    "github.com/TykTechnologies/midsommar/v2/proxy"
    "github.com/TykTechnologies/midsommar/v2/services"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func main() {
    // Database setup
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        log.Fatal("DATABASE_URL environment variable required")
    }
    
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }

    // Services setup
    service := services.NewService(db)
    budgetService := services.NewBudgetService(db, service)

    // Gateway setup
    port := 9090
    if portStr := os.Getenv("PORT"); portStr != "" {
        // Parse port if provided
    }
    
    gateway := aigateway.New(service, &proxy.Config{Port: port}, budgetService)

    // Graceful shutdown setup
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Start server
    go func() {
        log.Printf("Starting AI Gateway microproxy on :%d", port)
        if err := gateway.Start(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Gateway error: %v", err)
        }
    }()

    // Wait for shutdown
    <-ctx.Done()
    log.Println("Received shutdown signal...")

    // Graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := gateway.Stop(shutdownCtx); err != nil {
        log.Printf("Gateway shutdown error: %v", err)
        os.Exit(1)
    }
    
    log.Println("Gateway stopped gracefully")
}
```

## Environment Variables

The gateway respects these environment variables:

- `DEBUG_HTTP_PROXY=true` - Enables request/response logging for debugging
- `DATABASE_URL` - Database connection string (required)
- `PORT` - Server port (optional, defaults to config)

## Use Cases

1. **Standalone Microproxy**: Deploy as a dedicated AI gateway service
2. **API Gateway Integration**: Add AI capabilities to existing API gateways  
3. **Application Integration**: Embed AI gateway into web applications
4. **Development/Testing**: Local AI proxy for development environments
5. **Multi-tenant SaaS**: AI gateway with tenant isolation and billing

## Future Enhancements

- **Custom Configuration Providers**: Support for file-based, API-based configurations
- **Plugin System**: Extensible middleware and vendor support
- **Observability**: Enhanced metrics, tracing, and monitoring
- **Caching**: Request/response caching for improved performance
- **Load Balancing**: Multiple upstream instances for high availability
