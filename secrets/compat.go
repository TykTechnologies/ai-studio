// Package-level backwards-compatible wrappers around a global SecretStore.
//
// Deprecated: This file exists to avoid a 17-file refactor in a single PR.
// The long-term plan is to inject SecretStore directly into each consumer
// (services.Service, API handlers, gRPC server, etc.) via constructor
// parameters, eliminating the global defaultStore and this entire file.
//
// Migration path:
//   1. Add a SecretStore field to services.Service and pass it through NewServiceWithOCI.
//   2. Update service methods (llm_service, tool_service, datasource_service,
//      chat_service) to use the injected store instead of secrets.GetValue().
//   3. Update api/secrets_handlers.go to receive the store from the API struct.
//   4. Update grpc/control_server.go to receive the store at construction.
//   5. Update models/submission.go GORM hooks to use a store from context
//      or a package-level setter that can be removed later.
//   6. Once no callers remain, delete compat.go and the global defaultStore.
package secrets

import (
	"context"
	"fmt"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	defaultStore SecretStore
	defaultMu    sync.RWMutex
)

// SetStore sets the global default store directly.
func SetStore(store SecretStore) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultStore = store
}

// Store returns the current global default store, or nil.
func Store() SecretStore {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultStore
}

// SetDefaultStore is an alias for SetStore (backward compat).
func SetDefaultStore(store SecretStore) { SetStore(store) }

// DefaultStore is an alias for Store (backward compat).
func DefaultStore() SecretStore { return Store() }

// --- Backwards-compatible package-level wrappers ---

// SetDBRef initializes the default store with a DB-backed "database" implementation.
// The encryption key is read from the TYK_AI_SECRET_KEY environment variable.
// This preserves the original API — callers don't need to change.
func SetDBRef(db *gorm.DB) {
	rawKey := os.Getenv("TYK_AI_SECRET_KEY")
	store, err := NewStore("database", db, rawKey)
	if err != nil {
		fmt.Printf("secrets: failed to init default store via registry: %v\n", err)
		return
	}
	SetStore(store)
}

// EncryptValue encrypts a plaintext string using the application's AES key.
// Returns the encrypted value or the original if encryption fails or is not configured.
func EncryptValue(plaintext string) string {
	store := Store()
	if store == nil {
		return plaintext
	}

	encrypted, err := store.EncryptValue(context.Background(), plaintext)
	if err != nil {
		log.WithError(err).Warn("secrets: encryption failed, returning plaintext")
		return plaintext
	}
	return encrypted
}

// DecryptValue decrypts a value that was encrypted with EncryptValue.
// Returns the decrypted plaintext, or the original value if not encrypted.
func DecryptValue(value string) string {
	store := Store()
	if store == nil {
		return value
	}

	decrypted, err := store.DecryptValue(context.Background(), value)
	if err != nil {
		log.WithError(err).Warn("secrets: decryption failed, returning original value")
		return value
	}
	return decrypted
}

// GetValue resolves a reference ($SECRET/name, $ENV/name) to its value.
func GetValue(reference string, preserveRef bool) string {
	store := Store()
	if store == nil {
		return reference
	}
	return store.ResolveReference(context.Background(), reference, preserveRef)
}

// CreateSecret creates a new Secret record in the database.
// The db parameter is accepted for API compatibility but ignored — the default store is used.
func CreateSecret(_ *gorm.DB, settings *Secret) error {
	store := Store()
	if store == nil {
		return fmt.Errorf("secrets: no store initialized")
	}
	return store.Create(context.Background(), settings)
}

// GetSecretByID retrieves a Secret record from the database by ID.
func GetSecretByID(_ *gorm.DB, id uint, preserveRef bool) (*Secret, error) {
	store := Store()
	if store == nil {
		return nil, fmt.Errorf("secrets: no store initialized")
	}
	return store.GetByID(context.Background(), id, preserveRef)
}

// GetSecretByVarName retrieves a Secret record from the database by its name.
func GetSecretByVarName(_ *gorm.DB, name string, preserveRef bool) (*Secret, error) {
	store := Store()
	if store == nil {
		return nil, fmt.Errorf("secrets: no store initialized")
	}
	return store.GetByVarName(context.Background(), name, preserveRef)
}

// UpdateSecret updates an existing Secret record in the database.
func UpdateSecret(_ *gorm.DB, settings *Secret) error {
	store := Store()
	if store == nil {
		return fmt.Errorf("secrets: no store initialized")
	}
	return store.Update(context.Background(), settings)
}

// DeleteSecretByID deletes a Secret record from the database by ID.
func DeleteSecretByID(_ *gorm.DB, id uint) error {
	store := Store()
	if store == nil {
		return fmt.Errorf("secrets: no store initialized")
	}
	return store.Delete(context.Background(), id)
}

// ListSecrets lists secrets with pagination.
func ListSecrets(_ *gorm.DB, pageSize int, pageNumber int, all bool) ([]Secret, int64, int, error) {
	store := Store()
	if store == nil {
		return nil, 0, 0, fmt.Errorf("secrets: no store initialized")
	}
	return store.List(context.Background(), pageSize, pageNumber, all)
}

// GetOrCreateDefaultSecrets ensures default secrets exist in the database.
func GetOrCreateDefaultSecrets(_ *gorm.DB) error {
	store := Store()
	if store == nil {
		return fmt.Errorf("secrets: no store initialized")
	}
	return store.EnsureDefaults(context.Background(), []string{"OPENAI_KEY", "ANTHROPIC_KEY"})
}
