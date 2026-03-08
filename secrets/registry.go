package secrets

import (
	"fmt"
	"sync"
)

// KEKProviderFactory creates a KEKProvider from a raw encryption key.
// Each registered provider receives the same raw key and can derive/use it
// however it needs (e.g., SHA-256 for local, API key for Vault/KMS).
type KEKProviderFactory func(rawKey string) (KEKProvider, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]KEKProviderFactory{}
)

func init() {
	// Register the built-in local provider as the default.
	RegisterKEKProvider("local", func(rawKey string) (KEKProvider, error) {
		return NewLocalKEKProvider(rawKey), nil
	})
}

// RegisterKEKProvider registers a KEK provider factory under the given name.
// Panics if a provider with the same name is already registered.
func RegisterKEKProvider(name string, factory KEKProviderFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("secrets: KEK provider %q already registered", name))
	}
	registry[name] = factory
}

// KEKProviderNames returns the names of all registered KEK providers.
func KEKProviderNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// NewKEKProvider creates a KEK provider by name using the registered factory.
func NewKEKProvider(name, rawKey string) (KEKProvider, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("secrets: unknown KEK provider %q (registered: %v)", name, KEKProviderNames())
	}
	return factory(rawKey)
}
