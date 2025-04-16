/*
Package proxy provides a flexible LLM proxy for interacting with various language model providers.

The package has been redesigned with a focus on dependency inversion, allowing it to be:
1. Used within the Midsommar platform with concrete implementations
2. Used independently with custom implementations
3. Embedded in other applications with custom implementations

# Architecture

The proxy uses interface-based design with several key components:

## Core Interfaces

- ProxyServiceInterface: Combines domain-specific interfaces for LLMs, datasources, credentials, etc.
- BudgetServiceInterface: Handles budget enforcement and usage tracking
- AuthServiceInterface: Manages authentication
- ProxyDependencies: Composite interface that provides access to all required dependencies

## Adaptors

The package includes adaptor implementations that bridge between the concrete
implementations in the Midsommar platform and the interfaces required by the proxy.

## Usage Options

### 1. Standard Usage with Midsommar

```go
import (

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/auth"

)

	func main() {
	    service := services.NewService(db)
	    budgetService := service.Budget
	    authService := auth.NewAuthService(...)

	    // Option 1: Using the legacy constructor
	    proxyInstance := proxy.NewProxyLegacy(service, config, budgetService)

	    // Option 2: Using adaptors explicitly
	    serviceAdaptor := proxy.NewServiceAdaptor(service)
	    budgetAdaptor := proxy.NewBudgetServiceAdaptor(budgetService)
	    authAdaptor := proxy.NewAuthServiceAdaptor(authService)

	    proxyInstance := proxy.NewProxy(serviceAdaptor, config, budgetAdaptor, authAdaptor)

	    // Start the proxy
	    proxyInstance.Start()
	}

```

### 2. Embedded Usage with Custom Implementations

```go
import "github.com/TykTechnologies/midsommar/v2/proxy"

// Create custom implementations of the required interfaces

	type MyService struct {
	    // ...
	}

// Implement the required interface methods

	func (s *MyService) GetActiveLLMs() (models.LLMs, error) {
	    // Custom implementation
	}

// ... other implementations ...

	func main() {
	    // Create custom implementations
	    customService := &MyService{...}
	    customBudget := &MyBudgetService{...}
	    customAuth := &MyAuthService{...}

	    // Create dependencies container
	    deps := &MyDependencies{
	        service:      customService,
	        budgetService: customBudget,
	        authService:   customAuth,
	    }

	    // Create the proxy
	    proxyInstance := proxy.NewEmbeddedProxy(deps, config)

	    // Start the proxy
	    proxyInstance.Start()
	}

```

# Extension Points

The proxy can be extended in various ways:

1. Adding new vendor implementations via the `LLMVendorProvider` interface
2. Customizing authentication via the `AuthServiceInterface`
3. Implementing custom budget management via `BudgetServiceInterface`
4. Creating entirely custom service implementations via domain-specific interfaces

These extension points enable the proxy to work with various data sources,
authentication mechanisms, and LLM providers without requiring changes to the
core proxy code.
*/
package proxy
