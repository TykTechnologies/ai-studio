package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The failover waterfall must survive the snapshot -> SQLite -> runtime LLM
// round trip, because the shared proxy reads models.LLM.Failover and that is
// the only way an edge fails over the same way the hub does.
func TestEdgeSyncService_FailoverPersistedAndLoaded(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	now := timestamppb.New(time.Now())
	waterfall := `{"targets":[{"llm_id":2,"model":"claude-sonnet-4"}],"triggers":{"status_codes":[503],"on_timeout":false}}`
	snapshot := &pb.ConfigurationSnapshot{
		Version:      "1.0.0",
		SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "gpt", Slug: "gpt", Vendor: "openai", Endpoint: "https://x", IsActive: true, CreatedAt: now, UpdatedAt: now,
				Failover: waterfall},
			{Id: 2, Name: "claude", Slug: "claude", Vendor: "anthropic", Endpoint: "https://y", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
	}

	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	var llm database.LLM
	require.NoError(t, db.First(&llm, 1).Error)
	assert.JSONEq(t, waterfall, string(llm.Failover))
	var plain database.LLM
	require.NoError(t, db.First(&plain, 2).Error)
	assert.Empty(t, plain.Failover, "an LLM without a waterfall stays NULL")

	// ConvertToMidsommarLLM (management/CLI view) carries it typed.
	converted := ConvertToMidsommarLLM(llm)
	require.NotNil(t, converted.Failover)
	assert.Equal(t, uint(2), converted.Failover.Targets[0].LLMID)
	assert.Nil(t, ConvertToMidsommarLLM(plain).Failover)

	// And the shape the proxy reads is the typed models.LLMFailover.
	var f models.LLMFailover
	require.NoError(t, json.Unmarshal(llm.Failover, &f))
	assert.True(t, f.Enabled())
	trig := f.EffectiveTriggers()
	assert.False(t, trig.OnTimeout)
	assert.True(t, trig.StatusCodes[503])
	assert.False(t, trig.StatusCodes[502], "an explicit status list replaces the defaults")

	// The adapter is what hands LLMs to the shared proxy: the waterfall must
	// arrive there typed, on the runtime models.LLM.
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
		byName[l.Name] = l // Name carries the slug at runtime
	}
	require.Contains(t, byName, "gpt")
	assert.Equal(t, f, byName["gpt"].Failover)
	assert.False(t, byName["claude"].Failover.Enabled())
}
