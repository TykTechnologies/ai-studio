package openfga

import (
	"context"
	"os"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/authz"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// NewFromEnv creates an Authorizer based on the OPENFGA_ENABLED environment variable.
// When enabled ("true", "1", "yes"), it creates an embedded authorization store and runs FullSync.
// When disabled (default), it returns a NoopAuthorizer that defers to the legacy auth system.
func NewFromEnv(ctx context.Context, db *gorm.DB) (authz.Authorizer, error) {
	if !isEnabled() {
		log.Info().Msg("authz: fine-grained authorization disabled (set OPENFGA_ENABLED=true to enable)")
		return &authz.NoopAuthorizer{}, nil
	}

	store, err := New(ctx)
	if err != nil {
		return nil, err
	}

	if err := store.FullSync(ctx, db); err != nil {
		store.Close()
		return nil, err
	}

	return store, nil
}

func isEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OPENFGA_ENABLED")))
	return v == "true" || v == "1" || v == "yes"
}
