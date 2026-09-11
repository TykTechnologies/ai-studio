package grpc

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failover waterfall must ride the configuration snapshot to the edge
// as JSON, and be absent (not "{}") for LLMs that have none.
func TestGetConfigurationSnapshot_CarriesFailover(t *testing.T) {
	server, db := setupTestServer(t, nil)

	backup := &models.LLM{Name: "Backup", Vendor: models.ANTHROPIC, APIEndpoint: "https://y", Active: true}
	require.NoError(t, db.Create(backup).Error)
	primary := &models.LLM{
		Name: "Primary", Vendor: models.OPENAI, APIEndpoint: "https://x", Active: true,
		Failover: models.LLMFailover{
			Targets:  []models.LLMFailoverTarget{{LLMID: backup.ID, Model: "claude-sonnet-4"}},
			Triggers: &models.LLMFailoverTriggers{StatusCodes: []int{503, 429}},
		},
	}
	require.NoError(t, db.Create(primary).Error)

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)

	bySlug := map[string]*pb.LLMConfig{}
	for _, l := range snapshot.Llms {
		bySlug[l.Slug] = l
	}
	require.Contains(t, bySlug, "primary")
	require.Contains(t, bySlug, "backup")

	var got models.LLMFailover
	require.NoError(t, json.Unmarshal([]byte(bySlug["primary"].Failover), &got))
	assert.Equal(t, primary.Failover, got)
	assert.Equal(t, "", bySlug["backup"].Failover, "no waterfall serialises as empty, not {}")
}

// Changing the waterfall changes the checksum, which is what marks edges as
// pending a resync.
func TestSnapshotChecksum_FailoverIsContent(t *testing.T) {
	base := func() *pb.ConfigurationSnapshot {
		return &pb.ConfigurationSnapshot{Llms: []*pb.LLMConfig{{Id: 1, Name: "gpt", Vendor: "openai"}}}
	}
	baseline, err := ComputeSnapshotChecksum(base())
	require.NoError(t, err)

	with := base()
	with.Llms[0].Failover = `{"targets":[{"llm_id":2,"model":"claude-sonnet-4"}]}`
	c1, err := ComputeSnapshotChecksum(with)
	require.NoError(t, err)
	assert.NotEqual(t, baseline, c1)

	reordered := base()
	reordered.Llms[0].Failover = `{"targets":[{"llm_id":3,"model":"x"},{"llm_id":2,"model":"claude-sonnet-4"}]}`
	c2, err := ComputeSnapshotChecksum(reordered)
	require.NoError(t, err)
	assert.NotEqual(t, c1, c2, "rung order is content")
}
