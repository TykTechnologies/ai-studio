package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LLMInfo says whether the LLM has request filters, from the filters the
// repository loads with it, as the proxy decides.
func TestLLMInfoHasRequestFilters(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	createTestLLM(t, db, 1, "plain")
	createTestLLM(t, db, 2, "request-filtered")
	createTestLLM(t, db, 3, "response-filtered")
	reqFilter := &database.Filter{Name: "pii", Script: "x", ResponseFilter: false, IsActive: true}
	respFilter := &database.Filter{Name: "tone", Script: "x", ResponseFilter: true, IsActive: true}
	require.NoError(t, db.Create(reqFilter).Error)
	require.NoError(t, db.Create(respFilter).Error)
	require.NoError(t, db.Create(&database.LLMFilter{LLMID: 2, FilterID: reqFilter.ID}).Error)
	require.NoError(t, db.Create(&database.LLMFilter{LLMID: 3, FilterID: respFilter.ID}).Error)

	for id, want := range map[uint]bool{1: false, 2: true, 3: false} {
		llm, err := server.managementService.GetLLM(id)
		require.NoError(t, err)
		assert.Equal(t, want, server.convertLLMToPB(llm).GetHasRequestFilters(), "LLM %d", id)
	}
}
