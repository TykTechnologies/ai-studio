package database

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// API tokens are not stored. api_tokens.token_hash holds the token's
// credential hash (pkg/credhash, keyed with ENCRYPTION_KEY) and tokens are
// looked up by it; api_tokens.token, which used to hold the token itself,
// holds a redacted reference that identifies a token to an operator without
// being one. A token is shown once, when it is created. See
// features/ActiveActive.md, Phase 4 item 8.

// apiTokenHasher hashes API tokens for this process. A microgateway has one
// ENCRYPTION_KEY, so one hasher.
var apiTokenHasher atomic.Pointer[credhash.Hasher]

// ErrNoAPITokenHasher is returned when a token has to be hashed before
// SetAPITokenHasher was called. Nothing falls back to comparing tokens as
// stored: a gateway that cannot hash a token cannot check it.
var ErrNoAPITokenHasher = errors.New("API token hashing is not set up (ENCRYPTION_KEY)")

// SetAPITokenHasher sets the hasher API tokens are stored and looked up
// with; the service container makes it from ENCRYPTION_KEY at start-up.
func SetAPITokenHasher(h *credhash.Hasher) {
	apiTokenHasher.Store(h)
}

// SetAPITokenHasherFromKey is SetAPITokenHasher for an encryption key.
func SetAPITokenHasherFromKey(encryptionKey string) error {
	h, err := credhash.New(encryptionKey)
	if err != nil {
		return fmt.Errorf("API token hashing needs ENCRYPTION_KEY: %w", err)
	}
	SetAPITokenHasher(h)
	return nil
}

// HashAPIToken returns the hash a token is stored and looked up under.
func HashAPIToken(token string) (string, error) {
	h := apiTokenHasher.Load()
	if h == nil {
		return "", ErrNoAPITokenHasher
	}
	return h.Hash(token), nil
}

// redactedMark separates the two parts of a redacted token reference.
const redactedMark = "...redacted:"

// RedactedAPIToken is what api_tokens.token holds in place of a token: the
// start of the token (at most 8 characters, and at most a quarter of it),
// which is what logs show too, and part of its hash, which makes the value
// unique as the column requires. Neither authenticates anything.
func RedactedAPIToken(token, hash string) string {
	n := len(token) / 4
	if n > 8 {
		n = 8
	}
	tail := hash
	if len(tail) > 16 {
		tail = tail[len(tail)-16:]
	}
	return token[:n] + redactedMark + tail
}

// SetToken makes the row the one for token: its hash, and the redacted
// reference in place of the token.
func (t *APIToken) SetToken(token string) error {
	if token == "" {
		return errors.New("API token is empty")
	}
	hash, err := HashAPIToken(token)
	if err != nil {
		return err
	}
	t.TokenHash = hash
	t.Token = RedactedAPIToken(token, hash)
	return nil
}

// BeforeCreate hashes the token of a row that was built with the token
// itself, so no path stores one: after the insert, Token holds the redacted
// reference. A row that already has its hash (SetToken) is left as it is.
func (t *APIToken) BeforeCreate(tx *gorm.DB) error {
	if t.TokenHash != "" {
		return nil
	}
	return t.SetToken(t.Token)
}

// APITokenByValue selects the api_tokens row of a token: by its hash. The
// caller adds its own conditions (active, namespace) and runs the query.
func APITokenByValue(db *gorm.DB, token string) (*gorm.DB, string, error) {
	hash, err := HashAPIToken(token)
	if err != nil {
		return nil, "", err
	}
	return db.Where("token_hash = ?", hash), hash, nil
}

// HashStoredAPITokens converts the rows that still hold their token (written
// by a version before token hashing): each gets its hash, and the token is
// replaced by the redacted reference. It returns how many it converted. It
// is idempotent: converted rows are skipped, so it runs on every start, and
// two gateways on one database may run it at the same time.
//
// One-way: after it the tokens are not in the database, and a version from
// before token hashing, which compares tokens as stored, accepts none of
// them.
func HashStoredAPITokens(db *gorm.DB) (int, error) {
	if apiTokenHasher.Load() == nil {
		return 0, ErrNoAPITokenHasher
	}
	var rows []APIToken
	// Unscoped: soft-deleted rows hold tokens too.
	if err := db.Unscoped().Where("token_hash IS NULL OR token_hash = ''").Find(&rows).Error; err != nil {
		return 0, fmt.Errorf("read API tokens to hash: %w", err)
	}
	converted := 0
	for i := range rows {
		row := &rows[i]
		plain := row.Token
		if err := row.SetToken(plain); err != nil {
			return converted, fmt.Errorf("hash API token %d: %w", row.ID, err)
		}
		// Conditional on the token still being there: another gateway may
		// have converted the row since it was read.
		res := db.Unscoped().Model(&APIToken{}).
			Where("id = ? AND token = ? AND (token_hash IS NULL OR token_hash = '')", row.ID, plain).
			Updates(map[string]interface{}{"token_hash": row.TokenHash, "token": row.Token})
		if res.Error != nil {
			return converted, fmt.Errorf("store the hash of API token %d: %w", row.ID, res.Error)
		}
		converted += int(res.RowsAffected)
	}
	return converted, nil
}

// DropTokenCacheTable removes the token_cache table, which early versions
// created (keyed by the token itself) and nothing has written since.
func DropTokenCacheTable(db *gorm.DB) error {
	if !db.Migrator().HasTable("token_cache") {
		return nil
	}
	return db.Migrator().DropTable("token_cache")
}
