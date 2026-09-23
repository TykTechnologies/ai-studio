package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Edges resolve LLMs by the slug in the snapshot, while the proxy routes
// /llm/.../{slug}/ by slug.Make(name). If the two differ (names with
// punctuation), the edge's post-auth plugin lookup and model-router rewrites
// miss the LLM: post-auth plugins silently never run and routed requests 404.
func TestGetConfigurationSnapshot_LLMSlugMatchesProxyRouting(t *testing.T) {
	server, db := setupTestServer(t, nil)

	names := []string{"Mock GPT (host)", "GPT-4.1 / EU", "Claude's  Sonnet", "Plain"}
	for _, name := range names {
		require.NoError(t, db.Create(&models.LLM{Name: name, Vendor: models.OPENAI, APIEndpoint: "https://x", Active: true}).Error)
	}

	var routed models.LLM
	require.NoError(t, db.Where("name = ?", "Mock GPT (host)").First(&routed).Error)
	router := &models.ModelRouter{Name: "Router", Slug: "router", Active: true, Pools: []*models.ModelPool{{
		Name: "all", ModelPattern: "*",
		Vendors: []*models.PoolVendor{{LLMID: routed.ID, Weight: 1, Active: true}},
	}}}
	require.NoError(t, db.Create(router).Error)

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)

	byName := map[string]string{}
	for _, l := range snapshot.Llms {
		byName[l.Name] = l.Slug
	}
	for _, name := range names {
		assert.Equal(t, slug.Make(name), byName[name], "snapshot slug for %q must match proxy routing", name)
	}
	assert.Equal(t, "mock-gpt-host", byName["Mock GPT (host)"])

	require.Len(t, snapshot.ModelRouters, 1)
	vendor := snapshot.ModelRouters[0].Pools[0].Vendors[0]
	assert.Equal(t, "mock-gpt-host", vendor.LlmSlug, "router vendor slug must match proxy routing")
}
