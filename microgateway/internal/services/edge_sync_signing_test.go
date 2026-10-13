package services

import (
	"regexp"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The hub's resource signing keys (and the workspace pin and price
// multiplier overrides) must survive the snapshot -> SQLite ->
// runtime LLM round trip, so an edge signs and verifies native resource IDs
// with exactly the keys the hub and every other edge use.
func TestEdgeSyncService_ResourceSigningKeysPersistedAndLoaded(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	now := timestamppb.New(time.Now())
	prevExpires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	snapshot := &pb.ConfigurationSnapshot{
		Version:      "1.0.0",
		SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			// noopCryptoService: the "encrypted" values are the plaintext.
			{Id: 1, Name: "claude", Slug: "claude", Vendor: "anthropic", Endpoint: "https://x", IsActive: true,
				ApiKeyEncrypted: "vendor-key", CreatedAt: now, UpdatedAt: now,
				ResourceSigningSecretEncrypted:     "hub-secret",
				ResourceSigningSecretPrevEncrypted: models.ResourceSigningLegacyKey,
				ResourceSigningPrevExpiresAt:       timestamppb.New(prevExpires),
				VendorWorkspaceId:                  "wrkspc_pinned",
				PriceMultipliers:                   `{"batch":0.4}`},
			// An LLM from a hub that sends no secret.
			{Id: 2, Name: "old", Slug: "old", Vendor: "anthropic", Endpoint: "https://y", IsActive: true,
				ApiKeyEncrypted: "vendor-key-2", CreatedAt: now, UpdatedAt: now},
		},
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	var stored database.LLM
	require.NoError(t, db.First(&stored, 1).Error)
	assert.Equal(t, "hub-secret", stored.ResourceSigningSecretEncrypted)
	assert.Equal(t, models.ResourceSigningLegacyKey, stored.ResourceSigningSecretPrevEncrypted)
	require.NotNil(t, stored.ResourceSigningPrevExpiresAt)
	assert.True(t, stored.ResourceSigningPrevExpiresAt.Equal(prevExpires))

	repo := database.NewRepository(db)
	adapter := NewGatewayServiceAdapter(
		NewDatabaseGatewayService(db, repo),
		NewManagementService(db, repo, &noopCryptoService{}),
		nil, &noopCryptoService{},
		NewFilterService(db, repo), NewPluginService(db, repo),
		nil, db,
	)
	active, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	byName := map[string]models.LLM{}
	for _, l := range active {
		byName[l.Name] = l
	}
	require.Contains(t, byName, "claude")
	require.Contains(t, byName, "old")

	edgeLLM := byName["claude"]
	assert.Equal(t, "wrkspc_pinned", edgeLLM.VendorWorkspaceID, "the workspace pin reaches the proxy's LLM")
	assert.Empty(t, byName["old"].VendorWorkspaceID)
	assert.Equal(t, models.LLMPriceMultipliers{"batch": 0.4}, edgeLLM.PriceMultipliers, "the edge prices with the hub's overrides")
	assert.Nil(t, byName["old"].PriceMultipliers)
	var plain database.LLM
	require.NoError(t, db.First(&plain, 2).Error)
	assert.Empty(t, plain.PriceMultipliers, "no overrides stays NULL")
	keys := edgeLLM.ResourceSigningKeys()
	assert.Equal(t, "hub-secret", keys.Secret)
	assert.Equal(t, models.ResourceSigningLegacyKey, keys.Previous)
	assert.True(t, keys.PreviousActive(time.Now()))

	// The edge's codec is the hub's: same signing key, and it still accepts
	// IDs the vendor-credential key signed.
	edge, err := nativeapi.CodecForLLM(&edgeLLM, time.Now())
	require.NoError(t, err)
	hubKey, err := nativeapi.SecretKey("hub-secret")
	require.NoError(t, err)
	assert.Equal(t, hubKey.KID(), edge.KID())
	rt := nativeapi.ResourceType{Name: "anthropic.message_batch", IDCharset: regexp.MustCompile(`^msgbatch_[a-z0-9]+$`)}
	scope := nativeapi.Scope{LLMID: 1, AppID: 7}
	legacy, err := nativeapi.NewCodec("vendor-key")
	require.NoError(t, err)
	legacyID, err := legacy.Wrap(rt, "msgbatch_1", scope)
	require.NoError(t, err)
	got, err := edge.Unwrap(rt, legacyID, scope)
	require.NoError(t, err)
	assert.Equal(t, "msgbatch_1", got)

	// Without a secret from the hub the edge signs with the vendor-credential key.
	oldLLM := byName["old"]
	assert.Empty(t, oldLLM.ResourceSigningKeys().Secret)
	oldCodec, err := nativeapi.CodecForLLM(&oldLLM, time.Now())
	require.NoError(t, err)
	legacy2, err := nativeapi.NewCodec("vendor-key-2")
	require.NoError(t, err)
	assert.Equal(t, legacy2.KID(), oldCodec.KID())
	assert.NotEqual(t, legacy.KID(), edge.KID())
}
