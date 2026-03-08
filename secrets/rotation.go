package secrets

import (
	"context"
	"encoding/base64"
	"fmt"

	log "github.com/sirupsen/logrus"
)

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

	var offset int
	for {
		var batch []Secret
		if err := s.db.Model(&Secret{}).
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

			plaintext, err := decryptWith(ctx, oldCiphers, oldKey, secret.Value)
			if err != nil {
				log.Warnf("rotation: failed to decrypt secret %d (%s): %v", secret.ID, secret.VarName, err)
				result.Errors = append(result.Errors, RotationError{
					SecretID: secret.ID,
					VarName:  secret.VarName,
					Err:      err,
				})
				continue
			}

			encrypted, err := s.encryptValue(ctx, plaintext)
			if err != nil {
				log.Warnf("rotation: failed to re-encrypt secret %d (%s): %v", secret.ID, secret.VarName, err)
				result.Errors = append(result.Errors, RotationError{
					SecretID: secret.ID,
					VarName:  secret.VarName,
					Err:      err,
				})
				continue
			}

			if err := s.db.Model(secret).Update("value", encrypted).Error; err != nil {
				log.Warnf("rotation: failed to update secret %d (%s): %v", secret.ID, secret.VarName, err)
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

	result.Skipped = result.Total - result.Rotated - len(result.Errors)
	return result, nil
}

// RotateKEK re-wraps all encryption keys in the encryption_keys table with a new
// KEKProvider. The encrypted values in the secrets table are NOT touched — only the
// wrapped DEKs change. This is the fast path for KEK rotation.
func (s *Store) RotateKEK(ctx context.Context, oldKEK, newKEK KEKProvider) (*RotationResult, error) {
	result := &RotationResult{
		OldCipher: "v2",
		NewCipher: "v2",
	}

	ks := &gormKeyStore{db: s.db, kek: oldKEK}
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

		key.WrappedKey = base64.URLEncoding.EncodeToString(newWrapped)
		if err := s.db.Save(key).Error; err != nil {
			result.Errors = append(result.Errors, RotationError{
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
