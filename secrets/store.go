package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

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

// Store provides secret storage and encryption operations backed by a GORM database.
type Store struct {
	db       *gorm.DB
	rawKey   string
	ciphers  map[string]Cipher
	envelope *EnvelopeCipher
	kek      KEKProvider
}

// buildHistoricalKEKCache scans environment variables for historical KEKs.
// Format: TYK_AI_ENCRYPTION_KEY_2024_01=<passphrase> -> keyID "key-2024-01"
// This allows decryption of data encrypted with previous KEKs after rotation.
func buildHistoricalKEKCache(providerName string, rawKey string) map[string]KEKProvider {
	cache := make(map[string]KEKProvider)

	// Scan environment for historical KEKs
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TYK_AI_ENCRYPTION_KEY_") {
			continue
		}

		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		// Extract keyID from env var name
		// TYK_AI_ENCRYPTION_KEY_2024_01 -> key-2024-01
		envName := parts[0]
		suffix := envName[len("TYK_AI_ENCRYPTION_KEY_"):]

		// Skip the current key (without suffix)
		if suffix == "" || !strings.Contains(suffix, "_") {
			continue
		}

		keyID := "key-" + strings.ReplaceAll(suffix, "_", "-")
		historicalKey := parts[1]

		// Create provider for this historical KEK
		if providerName == "local" {
			// Import local package for type
			config := map[string]string{
				"RAW_KEY": historicalKey,
				"KEK_ID":  keyID,
			}
			provider, err := DefaultRegistry.Get("local", config)
			if err != nil {
				log.Warnf("Failed to load historical KEK %s: %v", keyID, err)
				continue
			}
			cache[keyID] = provider
			log.Debugf("Loaded historical KEK: %s", keyID)
		}
		// Add other providers (Vault, AWS KMS) here when implemented
	}

	return cache
}

// New creates a new DB-backed secret store using the "local" KEK provider.
// The rawKey is used both as the KEK passphrase and for decrypting
// legacy v1 secrets. New secrets are always written with envelope encryption.
// Requires that secrets/local has been imported (blank import) to register
// the "local" provider.
func New(db *gorm.DB, rawKey string) (*Store, error) {
	return NewFromProvider(db, rawKey, "local", nil)
}

// NewFromProvider creates a DB-backed secret store using the named KEK provider
// from the DefaultRegistry. The config map holds provider-specific settings
// collected from TYK_AI_<PROVIDER>_* env vars.
// Requires that the provider package has been imported to register its factory.
func NewFromProvider(db *gorm.DB, rawKey string, providerName string, config map[string]string) (*Store, error) {
	if config == nil {
		config = make(map[string]string)
	}
	config["RAW_KEY"] = rawKey

	// Create primary KEK
	kek, err := DefaultRegistry.Get(providerName, config)
	if err != nil {
		return nil, fmt.Errorf("KEK provider %q not available: %w (registered: %v)", providerName, err, DefaultRegistry.Names())
	}

	// Build historical KEK cache for rotation support
	kekCache := buildHistoricalKEKCache(providerName, rawKey)
	kekCache[kek.KeyID()] = kek // Add current KEK to cache

	if sc, ok := kek.(StartupChecker); ok {
		if err := sc.Startup(context.Background()); err != nil {
			return nil, fmt.Errorf("KEK provider %q startup check failed: %w", providerName, err)
		}
	}
	return NewWithKEKProvider(db, rawKey, kek, kekCache), nil
}

// NewWithKEKProvider creates a DB-backed secret store that uses envelope encryption (v2)
// with a custom KEKProvider (e.g., Vault, AWS KMS). New secrets are always written
// with envelope encryption. Legacy v1 secrets are read transparently.
// The kekCache holds historical KEK providers for decrypting data encrypted with previous KEKs.
func NewWithKEKProvider(db *gorm.DB, rawKey string, kek KEKProvider, kekCache map[string]KEKProvider) *Store {
	envelope := NewEnvelopeCipher(kek, kekCache)
	ciphers := legacyCipherInstances()
	ciphers["v2"] = envelope

	return &Store{
		db:       db,
		rawKey:   rawKey,
		ciphers:  ciphers,
		envelope: envelope,
		kek:      kek,
	}
}

// Close releases any resources held by the KEK provider.
// Safe to call even if the provider does not implement Shutdowner.
func (s *Store) Close(ctx context.Context) error {
	if sd, ok := s.kek.(Shutdowner); ok {
		return sd.Shutdown(ctx)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, secret *Secret) error {
	log.Debugf("[DEBUG] CreateSecret: Got key, length: %d", len(s.rawKey))

	encrypted, err := s.encryptValue(ctx, secret.Value)
	if err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to encrypt value: %v", err)
		return err
	}
	secret.Value = encrypted

	if err := s.db.Create(secret).Error; err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to create in DB: %v", err)
		return err
	}
	return nil
}

func (s *Store) GetByID(ctx context.Context, id uint, preserveRef bool) (*Secret, error) {
	var secret Secret
	if err := s.db.First(&secret, id).Error; err != nil {
		return nil, err
	}

	if preserveRef {
		secret.PreserveReference()
		return &secret, nil
	}

	decrypted, err := decryptWith(ctx, s.ciphers, s.rawKey, secret.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret %d: %w", id, err)
	}
	secret.Value = decrypted
	return &secret, nil
}

func (s *Store) GetByVarName(ctx context.Context, name string, preserveRef bool) (*Secret, error) {
	var secret Secret
	if err := s.db.Where("var_name = (?)", name).First(&secret).Error; err != nil {
		return nil, err
	}

	if preserveRef {
		secret.PreserveReference()
		return &secret, nil
	}

	decrypted, err := decryptWith(ctx, s.ciphers, s.rawKey, secret.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret %q: %w", name, err)
	}
	secret.Value = decrypted
	return &secret, nil
}

func (s *Store) Update(ctx context.Context, secret *Secret) error {
	// Only encrypt if the value is not already encrypted
	// (the API handler may pass an already-encrypted value when preserveRef=true)
	if !strings.HasPrefix(secret.Value, "$ENC/") {
		encrypted, err := s.encryptValue(ctx, secret.Value)
		if err != nil {
			return err
		}
		secret.Value = encrypted
	}

	return s.db.Save(secret).Error
}

func (s *Store) Delete(_ context.Context, id uint) error {
	return s.db.Delete(&Secret{}, id).Error
}

func (s *Store) List(_ context.Context, pageSize, pageNumber int, all bool) ([]Secret, int64, int, error) {
	var items []Secret
	var totalCount int64
	query := s.db.Model(&Secret{})

	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	if err := query.Find(&items).Error; err != nil {
		return nil, 0, 0, err
	}

	return items, totalCount, totalPages, nil
}

func (s *Store) EnsureDefaults(ctx context.Context, names []string) error {
	for _, name := range names {
		var count int64
		if err := s.db.Model(&Secret{}).Where("var_name = ?", name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			secret := &Secret{
				VarName: name,
				Value:   "",
			}
			if err := s.Create(ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) EncryptValue(ctx context.Context, plaintext string) (string, error) {
	return s.encryptValue(ctx, plaintext)
}

func (s *Store) DecryptValue(ctx context.Context, ciphertext string) (string, error) {
	return decryptWith(ctx, s.ciphers, s.rawKey, ciphertext)
}

func (s *Store) ResolveReference(ctx context.Context, reference string, preserveRef bool) string {
	if !strings.HasPrefix(reference, "$") {
		return reference
	}

	parts := strings.Split(reference, "/")
	if len(parts) != 2 {
		return reference
	}

	loc := parts[0]
	name := parts[1]

	switch loc {
	case "$ENV":
		return os.Getenv(name)
	case "$SECRET":
		if IsSecretReference(reference) && preserveRef {
			return reference
		}
		val, err := s.GetByVarName(ctx, name, preserveRef)
		if err != nil {
			log.Println(err)
			return reference
		}
		return val.Value
	default:
		return reference
	}
}

// encryptValue encrypts using envelope encryption (v2).
func (s *Store) encryptValue(ctx context.Context, plaintext string) (string, error) {
	return EncryptEnvelope(ctx, s.envelope, plaintext)
}
