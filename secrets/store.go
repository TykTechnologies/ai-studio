package secrets

import (
	"context"
	"fmt"
	"sync"
)

// SecretStore defines the interface for secret storage and encryption operations.
type SecretStore interface {
	Create(ctx context.Context, secret *Secret) error
	GetByID(ctx context.Context, id uint, preserveRef bool) (*Secret, error)
	GetByVarName(ctx context.Context, name string, preserveRef bool) (*Secret, error)
	Update(ctx context.Context, secret *Secret) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, pageSize, pageNumber int, all bool) ([]Secret, int64, int, error)
	EnsureDefaults(ctx context.Context, names []string) error

	EncryptValue(ctx context.Context, plaintext string) (string, error)
	DecryptValue(ctx context.Context, ciphertext string) (string, error)

	ResolveReference(ctx context.Context, reference string, preserveRef bool) string

	RotateKey(ctx context.Context, oldKey, newKey string) (*RotationResult, error)
}

// RotationResult reports the outcome of a key rotation operation.
type RotationResult struct {
	Total     int
	Rotated   int
	Skipped   int
	Errors    []RotationError
	OldCipher string
	NewCipher string
}

// RotationError records a per-secret rotation failure.
type RotationError struct {
	SecretID uint
	VarName  string
	Err      error
}

func (e RotationError) Error() string {
	return fmt.Sprintf("secret %d (%s): %v", e.SecretID, e.VarName, e.Err)
}

// StoreFactory is a function that creates a SecretStore from a database handle and raw key.
// The db parameter is intentionally interface{} — each backend type-asserts to what it needs
// (e.g., *gorm.DB for the database backend, nil for nop).
type StoreFactory func(db interface{}, rawKey string) (SecretStore, error)

// Registry holds named StoreFactory registrations.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]StoreFactory
}

// NewRegistry creates an empty store registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]StoreFactory),
	}
}

// Register adds a named store factory. Panics if the name is already taken.
func (r *Registry) Register(name string, factory StoreFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[name]; exists {
		panic(fmt.Sprintf("secrets: store %q already registered", name))
	}
	r.entries[name] = factory
}

// NewStore creates a SecretStore by name.
func (r *Registry) NewStore(name string, db interface{}, rawKey string) (SecretStore, error) {
	r.mu.RLock()
	factory, ok := r.entries[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("secrets: unknown store type %q (registered: %v)", name, r.names())
	}

	return factory(db, rawKey)
}

func (r *Registry) names() []string {
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	return names
}

// --- Global default registry (used by package-level helpers) ---

var defaultRegistry = NewRegistry()

// RegisterStore registers a named StoreFactory in the global registry.
// Typically called from init() in implementation sub-packages.
func RegisterStore(name string, factory StoreFactory) {
	defaultRegistry.Register(name, factory)
}

// NewStore creates a SecretStore by name using the global registry.
func NewStore(name string, db interface{}, rawKey string) (SecretStore, error) {
	return defaultRegistry.NewStore(name, db, rawKey)
}
