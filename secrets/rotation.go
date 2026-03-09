package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// parseV2KeyID extracts the key ID from a "$ENC/v2/<key_id>/..." value.
// Returns an error if the value is not v2 format.
func parseV2KeyID(value string) (uint, error) {
	if !strings.HasPrefix(value, "$ENC/v2/") {
		return 0, fmt.Errorf("not a v2 value")
	}
	rest := value[len("$ENC/v2/"):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return 0, fmt.Errorf("invalid v2 format")
	}
	id, err := strconv.ParseUint(rest[:slash], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid key id: %w", err)
	}
	return uint(id), nil
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

	// Invalidate cached DEKs since the underlying keys changed.
	s.envelope.ClearCache()

	result.Skipped = result.Total - result.Rotated - len(result.Errors)
	return result, nil
}

// rotateSecret decrypts a secret, generates a new DEK, updates the existing
// encryption_key row in place (stable key ID, bumped version), and re-encrypts.
func (s *Store) rotateSecret(ctx context.Context, tx *gorm.DB, oldCiphers map[string]Cipher, oldKey string, secret *Secret) error {
	plaintext, err := decryptWith(ctx, oldCiphers, oldKey, secret.Value)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	// Parse existing key ID from v2 secrets to update in place
	keyID, err := parseV2KeyID(secret.Value)
	if err != nil {
		// Legacy v1 secret — create a new key
		return s.rotateSecretNewKey(ctx, tx, secret, plaintext)
	}

	// Generate fresh DEK
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return fmt.Errorf("generate dek: %w", err)
	}

	// Wrap with current KEK
	wrapped, err := s.kek.WrapKey(ctx, dek)
	if err != nil {
		return fmt.Errorf("wrap dek: %w", err)
	}

	// Encrypt plaintext with new DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Update encryption_key row in place: new wrapped DEK, bumped version
	res := tx.Model(&EncryptionKey{}).
		Where("id = ?", keyID).
		Updates(map[string]interface{}{
			"wrapped_key": base64.URLEncoding.EncodeToString(wrapped),
			"version":     gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return fmt.Errorf("update key %d: %w", keyID, res.Error)
	}

	// Update secret value — key ID stays the same
	newValue := fmt.Sprintf("$ENC/v2/%d/%s", keyID, base64.URLEncoding.EncodeToString(ct))
	if err := tx.Model(secret).Update("value", newValue).Error; err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	return nil
}

// rotateSecretNewKey handles legacy (non-v2) secrets by creating a fresh key.
func (s *Store) rotateSecretNewKey(ctx context.Context, tx *gorm.DB, secret *Secret, plaintext string) error {
	txKS := &gormKeyStore{db: tx, kek: s.kek}
	txEnvelope := NewEnvelopeCipher(s.kek, txKS)

	encrypted, err := EncryptEnvelope(ctx, txEnvelope, plaintext)
	if err != nil {
		return fmt.Errorf("re-encrypt: %w", err)
	}

	if err := tx.Model(secret).Update("value", encrypted).Error; err != nil {
		return fmt.Errorf("update secret: %w", err)
	}
	return nil
}

// RotateKEK re-wraps all encryption keys in the encryption_keys table with a new
// KEKProvider. The encrypted values in the secrets table are NOT touched — only the
// wrapped DEKs change. This is the fast path for KEK rotation.
func (s *Store) RotateKEK(ctx context.Context, oldKEK, newKEK KEKProvider) (*RotationResult, error) {
	result := &RotationResult{
		OldCipher: "v2",
		NewCipher: "v2",
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		ks := &gormKeyStore{db: tx, kek: oldKEK}
		keys, err := ks.ListKeys(ctx)
		if err != nil {
			return fmt.Errorf("list encryption keys: %w", err)
		}

		for i := range keys {
			result.Total++
			key := &keys[i]

			// Decode the stored wrapped key
			wrappedDEK, err := base64.URLEncoding.DecodeString(key.WrappedKey)
			if err != nil {
				result.Errors = append(result.Errors, RotationError{
					SecretID: key.ID,
					VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
					Err:      fmt.Errorf("decode wrapped key: %w", err),
				})
				continue
			}

			// Unwrap with old KEK
			dek, err := oldKEK.UnwrapKey(ctx, wrappedDEK)
			if err != nil {
				result.Errors = append(result.Errors, RotationError{
					SecretID: key.ID,
					VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
					Err:      fmt.Errorf("unwrap with old kek: %w", err),
				})
				continue
			}

			// Re-wrap with new KEK
			newWrapped, err := newKEK.WrapKey(ctx, dek)
			if err != nil {
				result.Errors = append(result.Errors, RotationError{
					SecretID: key.ID,
					VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
					Err:      fmt.Errorf("wrap with new kek: %w", err),
				})
				continue
			}

			// Optimistic lock: only update if version hasn't changed
			oldVersion := key.Version
			res := tx.Model(key).
				Where("id = ? AND version = ?", key.ID, oldVersion).
				Updates(map[string]interface{}{
					"wrapped_key": base64.URLEncoding.EncodeToString(newWrapped),
					"version":     oldVersion + 1,
				})
			if res.Error != nil {
				result.Errors = append(result.Errors, RotationError{
					SecretID: key.ID,
					VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
					Err:      fmt.Errorf("update key: %w", res.Error),
				})
				continue
			}
			if res.RowsAffected == 0 {
				result.Errors = append(result.Errors, RotationError{
					SecretID: key.ID,
					VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
					Err:      fmt.Errorf("version conflict: key was modified concurrently"),
				})
				continue
			}

			result.Rotated++
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	result.Skipped = result.Total - result.Rotated - len(result.Errors)

	// Invalidate cached DEKs so the next operation re-fetches and unwraps
	// with whatever KEK the store currently holds.
	s.envelope.ClearCache()

	if h, ok := newKEK.(KeyRotatedHook); ok {
		if err := h.KeyRotated(ctx, result.Rotated, len(result.Errors)); err != nil {
			log.Warnf("key rotated hook: %v", err)
		}
	}

	return result, nil
}
