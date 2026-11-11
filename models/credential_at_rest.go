package models

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// credentialMigrationBatch bounds the rows read at a time.
var credentialMigrationBatch = 500

// CredentialMigration is what MigrateCredentialsAtRest did.
type CredentialMigration struct {
	// Migrated rows now have their lookup hash and, with an encryption
	// key, an encrypted secret.
	Migrated int
	// Skipped rows were left as they were: the secret does not decrypt
	// with the configured key, or another credential has the same secret.
	// They are logged, and found by nothing until that is put right.
	Skipped int
}

// MigrateCredentialsAtRest brings every credential row (soft-deleted ones
// included) to the form the Credential hooks write: secret_hash set to the
// current secrets.LookupHash of the secret and, when TYK_AI_SECRET_KEY is
// set, the secret encrypted. Rows from before the change have a plaintext
// secret and no hash; rows written while no key was set have a plaintext
// secret and an unkeyed hash.
//
// It is one-way: once it has run, code from before the change finds no
// credential by its secret and would show and send the ciphertext, which is
// why the schema's minimum reader version is raised with it.
//
// It is idempotent and resumable: only rows that are not in their final
// form are read, in batches by id, and each is written on its own with a
// compare-and-set on the secret that was read, so a second run, a run
// after an interruption, another replica's run and a key saved meanwhile
// through the API are all safe. Call it under the migration lock.
func MigrateCredentialsAtRest(db *gorm.DB) (CredentialMigration, error) {
	var res CredentialMigration
	db = db.Session(&gorm.Session{NewDB: true})
	prefix := secrets.LookupHashPrefix()
	encrypting := secrets.EncryptionKeyConfigured()

	pending := "(secret_hash IS NULL OR secret_hash NOT LIKE ?)"
	args := []any{prefix + "%"}
	if encrypting {
		pending = "(" + pending + " OR secret NOT LIKE ?)"
		args = append(args, encryptedPrefix+"%")
	}

	type row struct {
		ID         uint
		Secret     string
		SecretHash *string
	}
	var lastID uint
	for {
		var batch []row
		err := db.Table("credentials").Select("id", "secret", "secret_hash").
			Where("id > ? AND secret <> '' AND "+pending, append([]any{lastID}, args...)...).
			Order("id").Limit(credentialMigrationBatch).Find(&batch).Error
		if err != nil {
			return res, fmt.Errorf("read credentials: %w", err)
		}
		if len(batch) == 0 {
			return res, nil
		}
		for _, r := range batch {
			lastID = r.ID
			plain := secrets.DecryptValue(r.Secret)
			if isEncrypted(plain) {
				logger.Errorf("Credential %d: the stored secret does not decrypt with TYK_AI_SECRET_KEY (unset or changed); it is left as it is and validates nowhere", r.ID)
				res.Skipped++
				continue
			}
			stored := r.Secret
			if !isEncrypted(stored) {
				stored = secrets.EncryptValue(plain) // plain again when no key is set
			}
			done, err := setCredentialAtRest(db, r.ID, r.Secret, stored, secrets.LookupHash(plain))
			switch {
			case errors.Is(err, errDuplicateCredentialSecret):
				logger.Errorf("Credential %d has the same secret as another credential; it is left without a lookup hash and validates nowhere. Regenerate one of the two keys.", r.ID)
				res.Skipped++
			case err != nil:
				return res, fmt.Errorf("migrate credential %d: %w", r.ID, err)
			case done:
				res.Migrated++
			}
		}
	}
}

var errDuplicateCredentialSecret = errors.New("another credential has this secret")

// setCredentialAtRest writes one row's stored secret and hash, provided the
// row still holds the secret that was read. Two rows with one secret cannot
// both have its hash (it is unique): the later one is reported as
// errDuplicateCredentialSecret. That is looked for before writing, so that
// a caller's Postgres transaction is not aborted by the violation, and
// recognised from the write too, for a row hashed between the two.
func setCredentialAtRest(db *gorm.DB, id uint, read, stored, hash string) (bool, error) {
	var taken int64
	if err := db.Table("credentials").Where("secret_hash = ? AND id <> ?", hash, id).Count(&taken).Error; err != nil {
		return false, err
	}
	if taken > 0 {
		return false, errDuplicateCredentialSecret
	}
	res := db.Table("credentials").Where("id = ? AND secret = ?", id, read).
		UpdateColumns(map[string]any{"secret": stored, "secret_hash": hash})
	if res.Error != nil {
		if IsUniqueViolation(res.Error) {
			return false, errDuplicateCredentialSecret
		}
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
