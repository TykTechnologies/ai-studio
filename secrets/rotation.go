package secrets

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// parseV2KeyID extracts the keyID from a "$ENC/v2/${keyID}/..." value.
// Returns an error if the value is not v2 format.
func parseV2KeyID(value string) (string, error) {
	if !strings.HasPrefix(value, "$ENC/v2/") {
		return "", fmt.Errorf("not a v2 value")
	}
	rest := value[len("$ENC/v2/"):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", fmt.Errorf("invalid v2 format")
	}
	return rest[:slash], nil
}

const rotationBatchSize = 100

// RotateKey decrypts all secrets with oldKey and re-encrypts them with
// envelope encryption (v2). This migrates any legacy v1 secrets to v2.
func (s *Store) RotateKey(ctx context.Context, oldKey, _ string) (*RotationResult, error) {
	// Build old ciphers for decryption (v1 legacy + v2 envelope)
	oldCiphers := legacyCipherInstances()
	oldCiphers["v2"] = s.envelope

	result := &RotationResult{
		OldCipher: "all",
		NewCipher: "v2",
	}

	// Rotate in a transaction: for each secret, decrypt, generate a new DEK,
	// update the existing encryption_key row in place (same ID, bumped version),
	// and re-encrypt the secret. Key ID references in $ENC/v2/<key_id>/... stay stable.
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var offset int
		for {
			var batch []Secret
			if err := tx.Model(&Secret{}).
				Order("id ASC").
				Offset(offset).
				Limit(rotationBatchSize).
				Find(&batch).Error; err != nil {
				return fmt.Errorf("load batch at offset %d: %w", offset, err)
			}

			if len(batch) == 0 {
				break
			}

			for i := range batch {
				result.Total++
				secret := &batch[i]

				if err := s.rotateSecret(ctx, tx, oldCiphers, oldKey, secret); err != nil {
					log.Warnf("rotation: secret %d (%s): %v", secret.ID, secret.VarName, err)
					result.Errors = append(result.Errors, RotationError{
						SecretID: secret.ID,
						VarName:  secret.VarName,
						Err:      err,
					})
					continue
				}

				result.Rotated++
			}

			offset += len(batch)
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	result.Skipped = result.Total - result.Rotated - len(result.Errors)
	return result, nil
}

// rotateSecret decrypts a secret and re-encrypts it with the current KEK.
// With inline DEK storage, this simply re-encrypts with a fresh DEK.
func (s *Store) rotateSecret(ctx context.Context, tx *gorm.DB, oldCiphers map[string]Cipher, oldKey string, secret *Secret) error {
	plaintext, err := decryptWith(ctx, oldCiphers, oldKey, secret.Value)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	// Re-encrypt with current envelope cipher (inline DEK)
	encrypted, err := EncryptEnvelope(ctx, s.envelope, plaintext)
	if err != nil {
		return fmt.Errorf("re-encrypt: %w", err)
	}

	if err := tx.Model(secret).Update("value", encrypted).Error; err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	return nil
}

// RotateKEK is deprecated with inline DEK storage.
// KEK rotation is now automatic via the kekCache in EnvelopeCipher.
// Old data encrypted with previous KEKs is decrypted using historical
// KEK providers loaded from environment variables.
// To migrate data to a new KEK, use RotateKey() which re-encrypts all secrets.
func (s *Store) RotateKEK(ctx context.Context, oldKEK, newKEK KEKProvider) (*RotationResult, error) {
	log.Warn("RotateKEK is deprecated with inline DEK storage. Use RotateKey() to re-encrypt secrets with new KEK, or rely on historical KEK cache for automatic decryption.")
	return &RotationResult{
		OldCipher: "v2",
		NewCipher: "v2",
	}, nil
}
