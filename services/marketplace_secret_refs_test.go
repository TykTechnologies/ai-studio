package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Marketplace install carries no caller authorization, so it refuses secret
// references in the initial config.
func TestInstallFromMarketplaceRefusesSecretReferences(t *testing.T) {
	ms, _ := setupMarketplaceTest(t)
	_, err := ms.InstallFromMarketplace(context.Background(), &marketplace.InstallRequest{
		PluginID: "com.example.p", Version: "1.0.0",
		Config: map[string]interface{}{"token": "$SECRET/ANY"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret references are not accepted")
}
