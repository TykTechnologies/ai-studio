package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog/log"
)

// prepareAPITokenHashing sets the hasher for API tokens, converts the rows a
// version before token hashing wrote (the token replaced by its hash and a
// redacted reference) and removes the unused token_cache table. It runs on
// every start and does nothing once the rows are converted. A failure stops
// the start: the gateway could not check a token it has not hashed.
func prepareAPITokenHashing(db *gorm.DB, encryptionKey string) error {
	if err := database.SetAPITokenHasherFromKey(encryptionKey); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&database.APIToken{}, "token_hash") {
		// The schema is older than this binary (migrations are off): the
		// rows cannot be converted or looked up.
		return fmt.Errorf("api_tokens has no token_hash column: run the database migrations (-migrate, or DB_AUTO_MIGRATE=true) before starting this version")
	}
	converted, err := database.HashStoredAPITokens(db)
	if err != nil {
		return fmt.Errorf("hash stored API tokens: %w", err)
	}
	if converted > 0 {
		log.Warn().Int("tokens", converted).
			Msg("API tokens are now stored as hashes: the tokens themselves were removed from the database. They keep working; they can no longer be read back, and versions before 2.3 cannot use this database's tokens")
	}
	if err := database.DropTokenCacheTable(db); err != nil {
		log.Warn().Err(err).Msg("Could not remove the unused token_cache table")
	}
	return nil
}
