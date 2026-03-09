package captcha

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Provider verifies CAPTCHA tokens from the client.
type Provider interface {
	Verify(ctx context.Context, token, remoteIP string) error
	Name() string
	SiteKey() string
}

// ErrVerificationFailed is returned when the CAPTCHA token is invalid.
var ErrVerificationFailed = fmt.Errorf("captcha verification failed")

// IsVerificationError returns true if err wraps ErrVerificationFailed.
func IsVerificationError(err error) bool {
	return errors.Is(err, ErrVerificationFailed)
}

// FactoryFunc creates a Provider from application config values.
type FactoryFunc func(siteKey, secretKey string, opts map[string]string) (Provider, error)

var (
	mu       sync.RWMutex
	registry = map[string]FactoryFunc{}
)

// Register adds a provider factory to the global registry.
func Register(name string, factory FactoryFunc) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// NewProvider creates a provider by name from the registry.
func NewProvider(name, siteKey, secretKey string, opts map[string]string) (Provider, error) {
	mu.RLock()
	factory, ok := registry[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("captcha: unknown provider %q (registered: %v)", name, RegisteredProviders())
	}
	return factory(siteKey, secretKey, opts)
}

// RegisteredProviders returns a sorted list of registered provider names.
func RegisteredProviders() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
