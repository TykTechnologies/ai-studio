package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plugins see the gateway's slug for an LLM and whether it has request
// filters (not which ones), from ListLLMs and GetLLM alike.
func TestLLMInfoCarriesSlugAndRequestFilterPresence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	service := services.NewService(db)
	ctx := context.Background()

	requestFilter := &models.Filter{Name: "PII screen", Script: []byte("1"), ResponseFilter: false}
	responseFilter := &models.Filter{Name: "Output check", Script: []byte("1"), ResponseFilter: true}
	require.NoError(t, db.Create(requestFilter).Error)
	require.NoError(t, db.Create(responseFilter).Error)

	cases := []struct {
		name        string
		filters     []*models.Filter
		wantSlug    string
		wantFilters bool
	}{
		{"Claude Prod EU", []*models.Filter{responseFilter, requestFilter}, "claude-prod-eu", true},
		{"GPT Responses Only", []*models.Filter{responseFilter}, "gpt-responses-only", false},
		{"Plain Ollama", nil, "plain-ollama", false},
	}
	ids := map[string]uint32{}
	for _, tc := range cases {
		llm := &models.LLM{Name: tc.name, Vendor: models.OPENAI, APIEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4o", Active: true, Filters: tc.filters}
		require.NoError(t, llm.Create(db), tc.name)
		ids[tc.name] = uint32(llm.ID)
	}

	server := NewLLMManagementServer(service)
	list, err := server.ListLLMs(ctx, &pb.ListLLMsRequest{Page: 1, Limit: 100})
	require.NoError(t, err)
	byName := map[string]*pb.LLMInfo{}
	for _, l := range list.GetLlms() {
		byName[l.GetName()] = l
	}
	for _, tc := range cases {
		got := byName[tc.name]
		require.NotNil(t, got, tc.name)
		assert.Equal(t, tc.wantSlug, got.GetSlug(), "ListLLMs slug for %q", tc.name)
		assert.Equal(t, models.LLMRouteSlug(tc.name), got.GetSlug(), "the slug is the gateway's route slug")
		assert.Equal(t, tc.wantFilters, got.GetHasRequestFilters(), "ListLLMs has_request_filters for %q", tc.name)
		assert.NotNil(t, got.HasRequestFilters, "has_request_filters is always set, so absent means an older host")

		one, err := server.GetLLM(ctx, &pb.GetLLMRequest{LlmId: ids[tc.name]})
		require.NoError(t, err)
		assert.Equal(t, tc.wantSlug, one.GetLlm().GetSlug(), "GetLLM slug for %q", tc.name)
		assert.Equal(t, tc.wantFilters, one.GetLlm().GetHasRequestFilters(), "GetLLM has_request_filters for %q", tc.name)
	}
}

func TestLLMHasRequestFilters(t *testing.T) {
	assert.False(t, (&models.LLM{}).HasRequestFilters())
	assert.False(t, (&models.LLM{Filters: []*models.Filter{nil, {ResponseFilter: true}}}).HasRequestFilters())
	assert.True(t, (&models.LLM{Filters: []*models.Filter{{ResponseFilter: true}, {ResponseFilter: false}}}).HasRequestFilters())
}
