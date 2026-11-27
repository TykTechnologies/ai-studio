package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A standalone edge validates a Vertex LLM's endpoint and key as AI Studio
// does (vendors/vertex.ValidateEndpoint / ValidateKey).
func TestManagementService_VertexLLMValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	svc := NewManagementService(db, database.NewRepository(db), NewCryptoService("12345678901234567890123456789012"))
	const secret = "s3cr3t-material"

	for i, bad := range []string{"proj", "a:b:c", ""} {
		_, err := svc.CreateLLM(&CreateLLMRequest{Name: fmt.Sprintf("vx-bad-%d", i), Vendor: "vertex", Endpoint: bad, APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "project:location", bad)
	}

	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "vx-key", Vendor: "vertex", Endpoint: "my-proj:us-central1", APIKey: `{"type":"service_account","private_key":"` + secret + `"}`, DefaultModel: "gemini-2.5-flash"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)

	llm, err := svc.CreateLLM(&CreateLLMRequest{Name: "vx", Vendor: "vertex", Endpoint: "my-proj:us-central1", APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash", IsActive: true})
	require.NoError(t, err)
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "vx-global", Vendor: "vertex", Endpoint: "my-proj:global", DefaultModel: "gemini-2.5-flash"})
	require.NoError(t, err, "key-less is accepted with a warning")
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "vx-url", Vendor: "vertex", Endpoint: "https://us-central1-aiplatform.googleapis.com", APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
	require.NoError(t, err)
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "vx-local", Vendor: "vertex", Endpoint: "http://127.0.0.1:8089/v1/projects/p/locations/us-central1", APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
	require.NoError(t, err, "the fake-upstream URL form")
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "vx-us", Vendor: "vertex", Endpoint: "my-proj:us", APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
	require.NoError(t, err)
	for i, bad := range []string{"ftp://x", "https://"} {
		_, err := svc.CreateLLM(&CreateLLMRequest{Name: fmt.Sprintf("vx-badurl-%d", i), Vendor: "vertex", Endpoint: bad, APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
		require.Error(t, err, bad)
	}

	bad := "a:b:c"
	_, err = svc.UpdateLLM(llm.ID, &UpdateLLMRequest{Endpoint: &bad})
	require.Error(t, err)
	junk := "AIza" + secret
	_, err = svc.UpdateLLM(llm.ID, &UpdateLLMRequest{APIKey: &junk})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	name := "vx-renamed"
	_, err = svc.UpdateLLM(llm.ID, &UpdateLLMRequest{Name: &name})
	require.NoError(t, err, "an update that leaves endpoint and key alone is not re-checked")

	// An empty api_key on update keeps the stored key, so it is neither a
	// key-less change nor checked.
	empty := ""
	updated, err := svc.UpdateLLM(llm.ID, &UpdateLLMRequest{APIKey: &empty})
	require.NoError(t, err)
	assert.NotEmpty(t, updated.APIKeyEncrypted, "the stored key is kept")
}

// Config sync from the hub never runs the management API's validation: a
// snapshot carrying a Vertex LLM saved before the new rules (a malformed
// endpoint, no key) is applied like any other, so one old row cannot fail
// the whole sync.
func TestEdgeSyncService_LegacyVertexLLMApplied(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	now := timestamppb.New(time.Now())
	snapshot := &pb.ConfigurationSnapshot{
		Version:      "1.0.0",
		SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "legacy-vertex", Slug: "legacy-vertex", Vendor: "vertex", Endpoint: "a:b:c", IsActive: true, CreatedAt: now, UpdatedAt: now},
			{Id: 2, Name: "keyless-vertex", Slug: "keyless-vertex", Vendor: "vertex", Endpoint: "my-proj:us-central1", IsActive: true, CreatedAt: now, UpdatedAt: now},
			{Id: 3, Name: "gpt", Slug: "gpt", Vendor: "openai", Endpoint: "https://x", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))
	var n int64
	require.NoError(t, db.Model(&database.LLM{}).Count(&n).Error)
	assert.EqualValues(t, 3, n)
	var legacy database.LLM
	require.NoError(t, db.First(&legacy, 1).Error)
	assert.Equal(t, "a:b:c", legacy.Endpoint)
}
