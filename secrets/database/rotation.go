package database

import (
	"context"
	"encoding/base64"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

const rotationBatchSize = 100

// RotateKey decrypts all secrets with oldKey and re-encrypts them with the
// store's current encryption method (v1 or v2 envelope). For migrating
// v1 secrets to v2 envelope encryption, create the store with NewWithEnvelope
// and call RotateKey with the same key.
func (s *Database) RotateKey(ctx context.Context, oldKey, newKey string) (*secrets.RotationResult, error) {
	// Build old ciphers for decryption (includes v2 if envelope is configured)
	oldCiphers := secrets.LegacyCipherInstances()
	if s.envelope != nil {
		oldCiphers["v2"] = s.envelope
	}

	result := &secrets.RotationResult{
		OldCipher: "all",
		NewCipher: "v2",
	}
	if s.envelope == nil {
		result.NewCipher = "v1"
	}

	var offset int
	for {
		var batch []secrets.Secret
		if err := s.db.Model(&secrets.Secret{}).
			Order("id ASC").
			Offset(offset).
			Limit(rotationBatchSize).
			Find(&batch).Error; err != nil {
			return result, fmt.Errorf("load batch at offset %d: %w", offset, err)
		}

		if len(batch) == 0 {
			break
		}

		for i := range batch {
			result.Total++
			secret := &batch[i]

			plaintext, err := secrets.DecryptWith(ctx, oldCiphers, oldKey, secret.Value)
			if err != nil {
				log.Warnf("rotation: failed to decrypt secret %d (%s): %v", secret.ID, secret.VarName, err)
				result.Errors = append(result.Errors, secrets.RotationError{
					SecretID: secret.ID,
					VarName:  secret.VarName,
					Err:      err,
				})
				continue
			}

			var encrypted string
			if s.envelope != nil {
				encrypted, err = s.encryptValue(ctx, plaintext)
			} else {
				encrypted, err = secrets.EncryptWith(ctx, s.v1Cipher, newKey, plaintext)
			}
			if err != nil {
				log.Warnf("rotation: failed to re-encrypt secret %d (%s): %v", secret.ID, secret.VarName, err)
				result.Errors = append(result.Errors, secrets.RotationError{
					SecretID: secret.ID,
					VarName:  secret.VarName,
					Err:      err,
				})
				continue
			}

			if err := s.db.Model(secret).Update("value", encrypted).Error; err != nil {
				log.Warnf("rotation: failed to update secret %d (%s): %v", secret.ID, secret.VarName, err)
				result.Errors = append(result.Errors, secrets.RotationError{
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

	result.Skipped = result.Total - result.Rotated - len(result.Errors)
	return result, nil
}

// RotateKEK re-wraps all encryption keys in the encryption_keys table with a new
// KeyWrapper. The encrypted values in the secrets table are NOT touched — only the
// wrapped DEKs change. This is the fast path for KEK rotation.
func (s *Database) RotateKEK(ctx context.Context, oldWrapper, newWrapper secrets.KeyWrapper) (*secrets.RotationResult, error) {
	result := &secrets.RotationResult{
		OldCipher: "v2",
		NewCipher: "v2",
	}

	ks := &gormKeyStore{db: s.db, wrapper: oldWrapper}
	keys, err := ks.ListKeys(ctx)
	if err != nil {
		return result, fmt.Errorf("list encryption keys: %w", err)
	}

	for i := range keys {
		result.Total++
		key := &keys[i]

		// Decode the stored wrapped key
		wrappedDEK, err := base64.URLEncoding.DecodeString(key.WrappedKey)
		if err != nil {
			result.Errors = append(result.Errors, secrets.RotationError{
				SecretID: key.ID,
				VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
				Err:      fmt.Errorf("decode wrapped key: %w", err),
			})
			continue
		}

		// Unwrap with old KEK
		dek, err := oldWrapper.UnwrapKey(ctx, wrappedDEK)
		if err != nil {
			result.Errors = append(result.Errors, secrets.RotationError{
				SecretID: key.ID,
				VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
				Err:      fmt.Errorf("unwrap with old kek: %w", err),
			})
			continue
		}

		// Re-wrap with new KEK
		newWrapped, err := newWrapper.WrapKey(ctx, dek)
		if err != nil {
			result.Errors = append(result.Errors, secrets.RotationError{
				SecretID: key.ID,
				VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
				Err:      fmt.Errorf("wrap with new kek: %w", err),
			})
			continue
		}

		key.WrappedKey = base64.URLEncoding.EncodeToString(newWrapped)
		if err := s.db.Save(key).Error; err != nil {
			result.Errors = append(result.Errors, secrets.RotationError{
				SecretID: key.ID,
				VarName:  fmt.Sprintf("encryption_key_%d", key.ID),
				Err:      fmt.Errorf("update key: %w", err),
			})
			continue
		}

		result.Rotated++
	}

	result.Skipped = result.Total - result.Rotated - len(result.Errors)
	return result, nil
}
