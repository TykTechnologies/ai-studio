package secrets

import (
	"fmt"
	"sync"
)

// KEKProviderFactory creates a KEKProvider from provider-specific configuration.
// The config map contains env vars matching TYK_AI_<PROVIDER>_* with the prefix
// stripped, plus "RAW_KEY" injected by the store layer.
// For example, a Vault provider would receive {"ADDR": "...", "TOKEN": "..."},
// and the local provider reads config["RAW_KEY"] for its passphrase.
type KEKProviderFactory func(config map[string]string) (KEKProvider, error)

// ProviderRegistry holds named KEK provider factories.
// Providers register themselves via init() in their packages
// (e.g., secrets/local, secrets/vault).
type ProviderRegistry struct {
	mu        sync.RWMutex
	factories map[string]KEKProviderFactory
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		factories: make(map[string]KEKProviderFactory),
	}
}

// Register registers a KEK provider factory under the given name.
// If a provider with the same name already exists it is silently replaced
// (last writer wins), which allows tests and overrides to work naturally.
func (r *ProviderRegistry) Register(name string, factory KEKProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Get creates a KEK provider by name using the registered factory.
func (r *ProviderRegistry) Get(name string, config map[string]string) (KEKProvider, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("secrets: unknown KEK provider %q (registered: %v)", name, r.Names())
	}
	return factory(config)
}

// Names returns the names of all registered KEK providers.
func (r *ProviderRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the package-level registry used by NewFromProvider.
// Providers register themselves here via init() in their packages.
var DefaultRegistry = NewProviderRegistry()
