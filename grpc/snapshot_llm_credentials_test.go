package grpc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// LLM credentials are encrypted at rest with a random nonce (aa5), but the
// snapshot is built from what the LLM's hooks decrypt, so saving an
// unchanged Bedrock LLM with literal keys leaves the snapshot's checksum
// (and every edge's sync state) as it was, and the snapshot never carries
// the at-rest ciphertext.
func TestSnapshotChecksumStableAcrossSavesOfEncryptedCredentials(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	server, db := setupTestServer(t, nil)
	llm := &models.LLM{Name: "Bedrock", Vendor: models.BEDROCK, Active: true, APIEndpoint: "us-east-1",
		APIKey: "sk-fake-unit-test-key", Metadata: models.JSONMap{
			"aws_access_key_id":     "AKIAFAKEUNITTEST0000",
			"aws_secret_access_key": "fake/unit+test/secret/access/key",
			"aws_region":            "us-east-1",
		}}
	require.NoError(t, llm.Create(db))

	checksum := func() string {
		t.Helper()
		snap, err := server.getConfigurationSnapshot("")
		require.NoError(t, err)
		for _, l := range snap.Llms {
			assert.False(t, strings.Contains(l.Metadata, "$ENC/"), "the at-rest ciphertext never goes to an edge")
		}
		sum, err := ComputeSnapshotChecksum(snap)
		require.NoError(t, err)
		return sum
	}
	before := checksum()

	var loaded models.LLM
	require.NoError(t, db.First(&loaded, llm.ID).Error)
	require.NoError(t, loaded.Update(db))
	require.NoError(t, loaded.Update(db))
	assert.Equal(t, before, checksum(), "an unchanged save changes nothing an edge sees")

	loaded.Metadata["aws_secret_access_key"] = "another/fake/secret"
	require.NoError(t, loaded.Update(db))
	assert.NotEqual(t, before, checksum(), "a changed credential does")
}
