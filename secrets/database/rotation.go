package database

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

const rotationBatchSize = 100

// RotateKey decrypts all secrets with oldKey and re-encrypts them with newKey
// using the current default cipher (GCM v2). Partial failures do not abort —
// all secrets are attempted. Returns a RotationResult with counts and per-secret errors.
func (s *Database) RotateKey(ctx context.Context, oldKey, newKey string) (*secrets.RotationResult, error) {
	oldCiphers := secrets.AllCipherInstances()
	newCipher := secrets.DefaultCipherInstance()

	result := &secrets.RotationResult{
		OldCipher: "all",
		NewCipher: newCipher.Version(),
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

			encrypted, err := secrets.EncryptWith(ctx, newCipher, newKey, plaintext)
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
