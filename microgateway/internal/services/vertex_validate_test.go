package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A standalone edge validates a Vertex LLM's endpoint and key as AI Studio
// does (vendors/vertex.ValidateEndpoint / ValidateKey).
func TestManagementService_VertexLLMValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	svc := NewManagementService(db, database.NewRepository(db), NewCryptoService("12345678901234567890123456789012"))
	const secret = "s3cr3t-material"

	for _, bad := range []string{"proj", "a:b:c", ""} {
		_, err := svc.CreateLLM(&CreateLLMRequest{Name: "vx-bad", Vendor: "vertex", Endpoint: bad, APIKey: "ya29.t", DefaultModel: "gemini-2.5-flash"})
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

	assert.Error(t, ValidateCreateLLMRequest(&CreateLLMRequest{Name: "c", Vendor: "vertex", Endpoint: "proj", DefaultModel: "m"}))
	assert.NoError(t, ValidateCreateLLMRequest(&CreateLLMRequest{Name: "c", Vendor: "vertex", Endpoint: "proj-1:eu", DefaultModel: "m"}))
}
