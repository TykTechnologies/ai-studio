package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A standalone edge creates LLMs through ManagementService.CreateLLM. Its
// vendor validation rejected every vendor it did not list, so a System One
// (decision model) LLM could only reach an edge by hub sync.
func TestManagementService_CreateSystemOneLLM(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	svc := NewManagementService(db, database.NewRepository(db), NewCryptoService("12345678901234567890123456789012"))

	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "jev-hosted", Vendor: "systemone", DefaultModel: "jev-latest"})
	assert.ErrorContains(t, err, "API key is required", "the hosted API needs a key")

	llm, err := svc.CreateLLM(&CreateLLMRequest{Name: "jev", Vendor: "systemone", APIKey: "k", DefaultModel: "jev-latest", IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "systemone", llm.Vendor)

	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "kev-local", Vendor: "systemone", Endpoint: "http://llama:8080", DefaultModel: "kev-4b"})
	assert.NoError(t, err, "a self-host may run without a key")

	// The other vendors' rules are unchanged.
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "oai", Vendor: "openai", DefaultModel: "gpt-6-luna"})
	assert.ErrorContains(t, err, "API key is required for OpenAI")
	_, err = svc.CreateLLM(&CreateLLMRequest{Name: "nope", Vendor: "nope", DefaultModel: "m"})
	assert.ErrorContains(t, err, "unsupported vendor")
}
