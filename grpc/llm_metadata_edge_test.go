package grpc

import (
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test values, not credentials.
const (
	testAWSKeyID  = "AKIATESTKEYID0000000"
	testAWSSecret = "test-aws-secret-value-not-real"
)

func createBedrockLLM(t *testing.T, db *gorm.DB) *models.LLM {
	t.Helper()
	llm := &models.LLM{
		Name:         "Test Bedrock",
		Vendor:       models.BEDROCK,
		DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0",
		Active:       true,
		Metadata: models.JSONMap{
			"aws_access_key_id":     testAWSKeyID,
			"aws_secret_access_key": testAWSSecret,
			"aws_session_token":     "",
			"aws_region":            "eu-west-1",
		},
	}
	require.NoError(t, db.Create(llm).Error)
	return llm
}

func bedrockSnapshotLLM(t *testing.T, server *ControlServer) (*pb.ConfigurationSnapshot, *pb.LLMConfig) {
	t.Helper()
	snapshot, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	for _, l := range snapshot.Llms {
		if l.Name == "Test Bedrock" {
			return snapshot, l
		}
	}
	t.Fatal("the Bedrock LLM is not in the snapshot")
	return nil, nil
}

// Metadata credentials travel encrypted in metadata_secrets_encrypted; the
// metadata JSON has the refused-credential placeholder in their place, so an
// edge older than 2.3 fails closed. Other values are as before.
func TestSnapshotSendsMetadataCredentialsEncrypted(t *testing.T) {
	server, db := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	createBedrockLLM(t, db)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")

	_, l := bedrockSnapshotLLM(t, server)

	assert.NotContains(t, l.Metadata, testAWSKeyID, "credential in the plaintext metadata")
	assert.NotContains(t, l.Metadata, testAWSSecret, "credential in the plaintext metadata")
	assert.Contains(t, l.Metadata, `"aws_access_key_id":"`+secrets.RefusedCredential+`"`)
	assert.Contains(t, l.Metadata, `"aws_secret_access_key":"`+secrets.RefusedCredential+`"`)
	assert.Contains(t, l.Metadata, `"aws_region":"eu-west-1"`, "a non-credential value is sent as is")
	assert.Contains(t, l.Metadata, `"aws_session_token":""`, "an empty value is no credential")

	require.Len(t, l.MetadataSecretsEncrypted, 2)
	assert.Equal(t, testAWSKeyID, decryptLikeMicrogateway(t, key, l.MetadataSecretsEncrypted["aws_access_key_id"]))
	assert.Equal(t, testAWSSecret, decryptLikeMicrogateway(t, key, l.MetadataSecretsEncrypted["aws_secret_access_key"]))
	for k, v := range l.MetadataSecretsEncrypted {
		assert.False(t, strings.Contains(v, testAWSSecret) || strings.Contains(v, testAWSKeyID), "%s is not encrypted", k)
	}
}

// Regenerating the snapshot, or saving the LLM unchanged, keeps the checksum,
// so edges stay in sync, though each snapshot encrypts the credentials under
// a fresh nonce (the checksum covers a keyed digest of the plaintext).
func TestSnapshotChecksumStableWithMetadataCredentials(t *testing.T) {
	server, db := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	llm := createBedrockLLM(t, db)

	first, l1 := bedrockSnapshotLLM(t, server)

	var reloaded models.LLM
	require.NoError(t, db.First(&reloaded, llm.ID).Error)
	require.NoError(t, db.Save(&reloaded).Error)

	second, l2 := bedrockSnapshotLLM(t, server)
	assert.NotEqual(t, l1.MetadataSecretsEncrypted, l2.MetadataSecretsEncrypted, "random nonces")
	assert.NotEmpty(t, first.Checksum)
	assert.Equal(t, first.Checksum, second.Checksum)
}

// An LLM whose metadata credential cannot be encrypted is left out, as one
// whose API key cannot be.
func TestSnapshotExcludesLLMWhoseMetadataCredentialCannotBeEncrypted(t *testing.T) {
	server, db := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	createBedrockLLM(t, db)

	server.encryptionKey = "too-short"
	t.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "")

	snapshot, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	for _, l := range snapshot.Llms {
		assert.NotEqual(t, "Test Bedrock", l.Name)
		assert.NotContains(t, l.Metadata, testAWSSecret)
	}
}
