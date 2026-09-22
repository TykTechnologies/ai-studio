package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The edge executes filters in FilterIds order and stores the position as
// llm_filters.order_index, so the snapshot must carry the order the admin
// arranged, not the id order GORM's preload happens to return.
func TestGetConfigurationSnapshot_LLMFilterOrder(t *testing.T) {
	server, db := setupTestServer(t, nil)

	a := &models.Filter{Name: "A", Script: []byte(`output := {block: false}`)}
	b := &models.Filter{Name: "B", Script: []byte(`output := {block: false}`)}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	require.Greater(t, b.ID, a.ID)

	llm := &models.LLM{
		Name:        "Ordered",
		Vendor:      models.OPENAI,
		APIKey:      "k",
		APIEndpoint: "https://api.openai.com/v1",
		Active:      true,
		Filters:     []*models.Filter{b, a},
	}
	require.NoError(t, llm.Create(db))

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	require.Len(t, snapshot.Llms, 1)
	require.Equal(t, []uint32{uint32(b.ID), uint32(a.ID)}, snapshot.Llms[0].FilterIds)

	orderByID := map[uint32]int32{}
	for _, f := range snapshot.Filters {
		orderByID[f.Id] = f.OrderIndex
	}
	require.Equal(t, int32(0), orderByID[uint32(b.ID)], "B is first in the chain")
	require.Equal(t, int32(1), orderByID[uint32(a.ID)], "A is second in the chain")
}
