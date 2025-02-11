package providers

import (
	"fmt"
	"sync"
)

// Registry manages OpenAPI providers
type Registry struct {
	providers map[string]OpenAPIProvider
	mu        sync.RWMutex
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]OpenAPIProvider),
	}
}

// RegisterProvider adds a new provider to the registry
func (r *Registry) RegisterProvider(id string, provider OpenAPIProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider with ID %s already exists", id)
	}

	r.providers[id] = provider
	return nil
}

// GetProvider retrieves a provider by ID
func (r *Registry) GetProvider(id string) (OpenAPIProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[id]
	if !exists {
		return nil, fmt.Errorf("provider with ID %s not found", id)
	}

	return provider, nil
}

// ListProviders returns a list of all registered providers
func (r *Registry) ListProviders() []struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var providers []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	for id, provider := range r.providers {
		providers = append(providers, struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}{
			ID:          id,
			Name:        provider.Name(),
			Description: provider.Description(),
		})
	}

	return providers
}

// RemoveProvider removes a provider from the registry
func (r *Registry) RemoveProvider(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[id]; !exists {
		return fmt.Errorf("provider with ID %s not found", id)
	}

	delete(r.providers, id)
	return nil
}
