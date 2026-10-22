package analytics

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Several ingest requests record MCP calls at once (one per Pump), while
// the handler is stopped mid-flight. Under -race this must not race or
// panic, every row that lands must be whole (no row mixes two callers'
// values), and nothing may be written after Stop returns.
func TestDatabaseHandler_RecordMCPCallsConcurrent(t *testing.T) {
	db := mcpCallsDB(t)
	h := NewDatabaseHandler(context.Background(), db)

	const callers, batches, perBatch = 8, 20, 25
	var wg sync.WaitGroup
	start := make(chan struct{})
	for c := 0; c < callers; c++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			<-start
			for b := 0; b < batches; b++ {
				rows := make([]models.MCPCallRecord, perBatch)
				for i := range rows {
					rows[i] = models.MCPCallRecord{
						Timestamp:     time.Now(),
						ConnectionID:  uint(c + 1),
						TykAPIID:      fmt.Sprintf("api-%d", c+1),
						PrimitiveName: fmt.Sprintf("tool-%d-%d", c+1, b),
						Source:        models.MCPCallSourceTykPump,
					}
				}
				h.RecordMCPCalls(context.Background(), rows)
			}
		}(c)
	}
	close(start)

	// stop once rows are landing, while callers may still be sending
	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.MCPCallRecord{}).Count(&n)
		return n > 0
	}, 10*time.Second, 5*time.Millisecond)
	h.Stop()
	wg.Wait()

	var afterStop int64
	require.NoError(t, db.Model(&models.MCPCallRecord{}).Count(&afterStop).Error)
	time.Sleep(200 * time.Millisecond)
	var later int64
	require.NoError(t, db.Model(&models.MCPCallRecord{}).Count(&later).Error)
	assert.Equal(t, afterStop, later, "rows were written after Stop returned")
	assert.LessOrEqual(t, later, int64(callers*batches*perBatch))

	var rows []models.MCPCallRecord
	require.NoError(t, db.Find(&rows).Error)
	perBatchCount := map[string]int{}
	for _, r := range rows {
		assert.Equal(t, fmt.Sprintf("api-%d", r.ConnectionID), r.TykAPIID, "row %d mixes callers", r.ID)
		assert.Regexp(t, fmt.Sprintf(`^tool-%d-\d+$`, r.ConnectionID), r.PrimitiveName)
		perBatchCount[r.PrimitiveName]++
	}
	// a batch is written whole or not at all
	for name, n := range perBatchCount {
		assert.Equal(t, perBatch, n, "batch %s was split", name)
	}
}
