package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	// Skipped rows cannot be found by their secret: the secret does not
	// decrypt with the configured key, or another credential has the same
	// secret and keeps its hash. They are logged.
	Skipped int
}

// MigrateCredentialsAtRest brings every credential row (soft-deleted ones
// included) to the form the Credential hooks write: secret_hash set to the
// current secrets.LookupHash of the secret and, when TYK_AI_SECRET_KEY is
// set, the secret sealed under the database's data key in the form this
// process writes. It converts rows from before the change (a plaintext
// secret, no hash), rows written while no key was set (plaintext, unkeyed
// hash), rows in the "$ENC/" form an earlier revision of this change wrote
// on development stacks, and, in a FIPS-mode process, rows whose data key
// was derived with scrypt.
//
// It is one-way: once it has run, code from before the change finds no
// credential by its secret and would show and send the ciphertext, which is
// why the schema's minimum reader version is raised with it.
//
// It is idempotent and resumable: only rows that are not in their final
// form are read, in batches by id, and each is written on its own with a
// compare-and-set on the secret that was read, so a second run, a run
// after an interruption, another replica's run and a key saved meanwhile
// through the API are all safe. It derives one key however many rows there
// are (a row in the "$ENC/" form costs a derivation of its own to read).
// Call it under the migration lock, after EnsureDataKeySalt.
func MigrateCredentialsAtRest(db *gorm.DB) (CredentialMigration, error) {
	var res CredentialMigration
	db = db.Session(&gorm.Session{NewDB: true})
	hashPrefix := secrets.LookupHashPrefix()
	encrypting := secrets.EncryptionKeyConfigured()

	pending := "(secret_hash IS NULL OR secret_hash NOT LIKE ?)"
	args := []any{hashPrefix + "%"}
	if encrypting {
		pending = "(" + pending + " OR secret NOT LIKE ?)"
		args = append(args, secrets.DataKeyCurrentPrefix()+"%")
	}

	type row struct {
		ID         uint
		KeyID      string
		Secret     string
		SecretHash *string
		Active     bool
		DeletedAt  *time.Time
	}
	var lastID uint
	for {
		var batch []row
		err := db.Table("credentials").Select("id", "key_id", "secret", "secret_hash", "active", "deleted_at").
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
			plain, ok := openCredentialSecret(db, r.KeyID, r.Secret)
			if !ok {
				logger.Errorf("Credential %d: the stored secret cannot be read (TYK_AI_SECRET_KEY unset or changed, or the database's data key salt is missing); it is left as it is and validates nowhere", r.ID)
				res.Skipped++
				continue
			}
			stored := r.Secret
			if !encrypting || !strings.HasPrefix(stored, secrets.DataKeyCurrentPrefix()) {
				if stored, err = sealCredentialSecret(db, r.KeyID, plain); err != nil {
					return res, fmt.Errorf("encrypt credential %d: %w", r.ID, err)
				}
			}
			hash := secrets.LookupHash(plain)
			done, err := setCredentialAtRest(db, r.ID, r.Secret, stored, hash)
			if errors.Is(err, errDuplicateCredentialSecret) {
				// Two rows with one secret: only one can be found by it.
				usable := r.Active && r.DeletedAt == nil
				loser, lerr := settleDuplicateSecret(db, r.ID, usable, r.Secret, stored, hash)
				if lerr != nil {
					return res, fmt.Errorf("migrate credential %d: %w", r.ID, lerr)
				}
				logger.Errorf("Credentials %d and another have the same secret; credential %d is left without a lookup hash and validates nowhere. Regenerate its key.", r.ID, loser)
				res.Skipped++
				continue
			}
			switch {
			case err != nil:
				return res, fmt.Errorf("migrate credential %d: %w", r.ID, err)
			case done:
				res.Migrated++
			}
		}
	}
}

var errDuplicateCredentialSecret = errors.New("another credential has this secret")

// settleDuplicateSecret decides which of two rows with one secret keeps its
// lookup hash, and returns the id of the one that does not. Before secrets
// were hashed, a key shared by two rows validated as the first row, by id,
// that was live (not soft-deleted) and active: that row keeps validating.
// So the row being migrated takes the hash from its holder only when it is
// usable and the holder is not; otherwise the holder, which has the lower
// id in a migration by id, keeps it. The loser's secret is still brought to
// its stored form, without a hash.
func settleDuplicateSecret(db *gorm.DB, id uint, usable bool, read, stored, hash string) (loser uint, err error) {
	var holder struct {
		ID        uint
		Active    bool
		DeletedAt *time.Time
	}
	if err := db.Table("credentials").Select("id", "active", "deleted_at").Where("secret_hash = ? AND id <> ?", hash, id).Take(&holder).Error; err != nil {
		return 0, err
	}
	holderUsable := holder.Active && holder.DeletedAt == nil
	if !usable || holderUsable {
		err := db.Table("credentials").Where("id = ? AND secret = ?", id, read).
			UpdateColumns(map[string]any{"secret": stored, "secret_hash": nil}).Error
		return id, err
	}
	return holder.ID, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("credentials").Where("id = ? AND secret_hash = ?", holder.ID, hash).
			UpdateColumn("secret_hash", nil).Error; err != nil {
			return err
		}
		return tx.Table("credentials").Where("id = ? AND secret = ?", id, read).
			UpdateColumns(map[string]any{"secret": stored, "secret_hash": hash}).Error
	})
}

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
