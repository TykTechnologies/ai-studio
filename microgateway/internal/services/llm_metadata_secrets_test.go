package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Test values, not credentials.
const (
	metadataSecretsTestKey = "0123456789abcdef0123456789abcdef"
	testAWSKeyID           = "AKIATESTKEYID0000000"
	testAWSSecret          = "test-aws-secret-value-not-real"
)

// activeLLMs is the part of the gateway adapter these tests use.
type activeLLMs interface {
	GetActiveLLMs() ([]models.LLM, error)
}

func metadataSecretsAdapter(t *testing.T, crypto CryptoServiceInterface) (activeLLMs, *gorm.DB, *database.Repository) {
	t.Helper()
	db := setupToolsSyncTestDB(t)
	repo := database.NewRepository(db)
	adapter := NewGatewayServiceAdapter(
		NewDatabaseGatewayService(db, repo),
		NewManagementService(db, repo, crypto),
		nil, crypto,
		NewFilterService(db, repo), NewPluginService(db, repo),
		nil, db,
	)
	return adapter, db, repo
}

// The hub's snapshot (credentials in metadata_secrets_encrypted, the
// refused-credential placeholder in metadata) is stored as it came: the edge
// DB holds no credential in plaintext, and the LLM the gateway builds has
// the values.
func TestEdgeSync_MetadataCredentialsStoredEncryptedAndUsed(t *testing.T) {
	crypto := NewCryptoService(metadataSecretsTestKey)
	adapter, db, _ := metadataSecretsAdapter(t, crypto)

	encKeyID, err := crypto.Encrypt(testAWSKeyID)
	require.NoError(t, err)
	encSecret, err := crypto.Encrypt(testAWSSecret)
	require.NoError(t, err)
	now := timestamppb.New(time.Now())
	snapshot := &pb.ConfigurationSnapshot{
		Version: "1.0.0", SnapshotTime: now,
		Llms: []*pb.LLMConfig{{
			Id: 1, Name: "bedrock", Slug: "bedrock", Vendor: "bedrock", IsActive: true, CreatedAt: now, UpdatedAt: now,
			Metadata: `{"aws_access_key_id":"` + secrets.RefusedCredential + `","aws_secret_access_key":"` + secrets.RefusedCredential + `","aws_region":"eu-west-1"}`,
			MetadataSecretsEncrypted: map[string]string{
				"aws_access_key_id":     encKeyID,
				"aws_secret_access_key": encSecret,
			},
		}},
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	// At rest: no plaintext anywhere in the row.
	var row map[string]interface{}
	require.NoError(t, db.Table("llms").Where("id = ?", 1).Take(&row).Error)
	for col, v := range row {
		s, _ := v.(string)
		if b, ok := v.([]byte); ok {
			s = string(b)
		}
		assert.NotContains(t, s, testAWSKeyID, "plaintext credential in column %s", col)
		assert.NotContains(t, s, testAWSSecret, "plaintext credential in column %s", col)
	}

	// An edge that reads only the metadata (older than 2.3) gets the placeholder.
	var stored database.LLM
	require.NoError(t, db.First(&stored, 1).Error)
	var plain map[string]interface{}
	require.NoError(t, json.Unmarshal(stored.Metadata, &plain))
	assert.Equal(t, secrets.RefusedCredential, plain["aws_secret_access_key"])

	// The gateway's LLM has the values.
	active, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, testAWSKeyID, active[0].Metadata["aws_access_key_id"])
	assert.Equal(t, testAWSSecret, active[0].Metadata["aws_secret_access_key"])
	assert.Equal(t, "eu-west-1", active[0].Metadata["aws_region"])
}

// A credential removed on the hub (a snapshot with no
// metadata_secrets_encrypted) is removed on the edge too: the next sync
// clears the column, so no stale ciphertext is laid over the metadata.
func TestEdgeSync_RemovedMetadataCredentialsAreCleared(t *testing.T) {
	crypto := NewCryptoService(metadataSecretsTestKey)
	adapter, db, _ := metadataSecretsAdapter(t, crypto)

	encSecret, err := crypto.Encrypt(testAWSSecret)
	require.NoError(t, err)
	now := timestamppb.New(time.Now())
	llm := func(metadata string, secretsEnc map[string]string) *pb.ConfigurationSnapshot {
		return &pb.ConfigurationSnapshot{
			Version: "1.0.0", SnapshotTime: now,
			Llms: []*pb.LLMConfig{{
				Id: 1, Name: "bedrock", Slug: "bedrock", Vendor: "bedrock", IsActive: true, CreatedAt: now, UpdatedAt: now,
				Metadata: metadata, MetadataSecretsEncrypted: secretsEnc,
			}},
		}
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(llm(
		`{"aws_secret_access_key":"`+secrets.RefusedCredential+`","aws_region":"eu-west-1"}`,
		map[string]string{"aws_secret_access_key": encSecret})))
	var stored database.LLM
	require.NoError(t, db.First(&stored, 1).Error)
	require.NotEmpty(t, stored.MetadataSecretsEncrypted)

	// The hub no longer has the credential.
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(llm(`{"aws_region":"eu-west-1"}`, nil)))
	stored = database.LLM{}
	require.NoError(t, db.First(&stored, 1).Error)
	assert.Empty(t, stored.MetadataSecretsEncrypted, "stale metadata credentials survived the sync")

	active, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.NotContains(t, active[0].Metadata, "aws_secret_access_key")
	assert.Equal(t, "eu-west-1", active[0].Metadata["aws_region"])
}

// A credential that does not decrypt, or is empty, is refused: never the
// ciphertext, never an empty value a vendor client would fill from its own
// environment.
func TestOverlayLLMMetadataSecrets_RefusesWhatDoesNotDecrypt(t *testing.T) {
	crypto := NewCryptoService(metadataSecretsTestKey)
	other := NewCryptoService("ffffffffffffffffffffffffffffffff")
	wrongKey, err := other.Encrypt(testAWSSecret)
	require.NoError(t, err)
	column, err := json.Marshal(map[string]string{"aws_secret_access_key": wrongKey, "aws_session_token": ""})
	require.NoError(t, err)

	out := overlayLLMMetadataSecrets(crypto, 1, models.JSONMap{"aws_region": "us-east-1"}, column)
	assert.Equal(t, secrets.RefusedCredential, out["aws_secret_access_key"])
	assert.Equal(t, secrets.RefusedCredential, out["aws_session_token"])
	assert.Equal(t, "us-east-1", out["aws_region"])

	// No column: metadata is used as stored (an LLM synced before 2.3).
	plain := models.JSONMap{"aws_region": "us-east-1"}
	assert.Equal(t, plain, overlayLLMMetadataSecrets(crypto, 1, plain, nil))

	// An unreadable column refuses every credential key.
	out = overlayLLMMetadataSecrets(crypto, 1, models.JSONMap{"aws_secret_access_key": testAWSSecret}, []byte("{not json"))
	assert.Equal(t, secrets.RefusedCredential, out["aws_secret_access_key"])
}

// A standalone edge's own writes split metadata the same way, a credential
// sent back redacted keeps its value, and the admin API shows it redacted.
func TestManagementService_LLMMetadataCredentials(t *testing.T) {
	crypto := NewCryptoService(metadataSecretsTestKey)
	adapter, db, repo := metadataSecretsAdapter(t, crypto)
	mgmt := NewManagementService(db, repo, crypto)

	created, err := mgmt.CreateLLM(&CreateLLMRequest{
		Name: "standalone-llm", Vendor: "vertex", Endpoint: "https://vertex.example", IsActive: true, DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0",
		Metadata: map[string]interface{}{
			"aws_access_key_id":     testAWSKeyID,
			"aws_secret_access_key": testAWSSecret,
			"aws_region":            "eu-west-1",
			"aws_session_token":     "$SECRET/AWS_TOKEN",
		},
	})
	require.NoError(t, err)
	assert.NotContains(t, string(created.Metadata), testAWSSecret)
	assert.NotContains(t, string(created.Metadata), testAWSKeyID)
	assert.Contains(t, string(created.Metadata), "$SECRET/AWS_TOKEN", "a reference is no credential value")

	shown := RedactLLMForResponse(created)
	var shownMeta map[string]interface{}
	require.NoError(t, json.Unmarshal(shown.Metadata, &shownMeta))
	assert.Equal(t, models.CredentialRedacted, shownMeta["aws_secret_access_key"])
	assert.Equal(t, "eu-west-1", shownMeta["aws_region"])
	encoded, err := json.Marshal(shown)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "metadata_secrets_encrypted", "the encrypted column is not in the API")

	// Saving back what the API showed keeps the credentials; a new region
	// is written.
	shownMeta["aws_region"] = "us-west-2"
	_, err = mgmt.UpdateLLM(created.ID, &UpdateLLMRequest{Metadata: shownMeta})
	require.NoError(t, err)

	active, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, testAWSKeyID, active[0].Metadata["aws_access_key_id"])
	assert.Equal(t, testAWSSecret, active[0].Metadata["aws_secret_access_key"])
	assert.Equal(t, "us-west-2", active[0].Metadata["aws_region"])
}
