package grpc

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// The snapshot carries each LLM's resource signing keys encrypted for the
// edge, so every edge signs native resource IDs with the hub's key. A
// rotation changes the checksum (edges go pending); regenerating does not.
func TestSnapshot_ResourceSigningKeys(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")

	byID := func(s *pb.ConfigurationSnapshot, id uint) *pb.LLMConfig {
		for _, l := range s.Llms {
			if uint(l.Id) == id {
				return l
			}
		}
		t.Fatalf("LLM %d not in snapshot", id)
		return nil
	}
	stored := func(id uint) models.ResourceSigningKeys {
		var l models.LLM
		require.NoError(t, db.First(&l, id).Error)
		return l.ResourceSigningKeys()
	}

	first, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	pbLLM := byID(first, llms[1].ID)
	require.NotEmpty(t, pbLLM.ResourceSigningSecretEncrypted)
	assert.Equal(t, stored(llms[1].ID).Secret, decryptLikeMicrogateway(t, key, pbLLM.ResourceSigningSecretEncrypted))
	assert.Empty(t, pbLLM.ResourceSigningSecretPrevEncrypted)
	assert.Nil(t, pbLLM.ResourceSigningPrevExpiresAt)
	assert.Empty(t, pbLLM.VendorWorkspaceId)

	again, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Equal(t, first.Checksum, again.Checksum)

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, models.RotateLLMResourceSigningSecret(db, llms[1].ID, time.Hour, now))
	rotated, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.NotEqual(t, first.Checksum, rotated.Checksum, "a rotation must mark edges pending")
	pbLLM = byID(rotated, llms[1].ID)
	keys := stored(llms[1].ID)
	assert.Equal(t, keys.Secret, decryptLikeMicrogateway(t, key, pbLLM.ResourceSigningSecretEncrypted))
	assert.Equal(t, keys.Previous, decryptLikeMicrogateway(t, key, pbLLM.ResourceSigningSecretPrevEncrypted))
	require.NotNil(t, pbLLM.ResourceSigningPrevExpiresAt)
	assert.True(t, pbLLM.ResourceSigningPrevExpiresAt.AsTime().Equal(now.Add(time.Hour)))
}

// The LLM's workspace pin travels with its config so edges pin it too.
func TestSnapshot_VendorWorkspaceID(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llms[1].ID).Update("vendor_workspace_id", "wrkspc_pinned").Error)
	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	got := map[uint32]string{}
	for _, l := range snap.Llms {
		got[l.Id] = l.VendorWorkspaceId
	}
	assert.Equal(t, "wrkspc_pinned", got[uint32(llms[1].ID)])
	assert.Equal(t, "", got[uint32(llms[0].ID)])
}

// The LLM's price multiplier overrides travel with its config: the edge
// prices its own usage.
func TestSnapshot_PriceMultipliers(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llms[1].ID).
		Update("price_multipliers", models.LLMPriceMultipliers{"batch": 0.4}).Error)
	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	got := map[uint32]string{}
	for _, l := range snap.Llms {
		got[l.Id] = l.PriceMultipliers
	}
	assert.JSONEq(t, `{"batch":0.4}`, got[uint32(llms[1].ID)])
	assert.Equal(t, "", got[uint32(llms[0].ID)], "none is empty, so the checksum only moves when overrides exist")
}
