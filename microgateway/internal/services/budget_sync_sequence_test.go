package services

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// A sync whose sequence is below the last one applied is ignored, as
// before, and now reported at WARN: once per stream, not once per sync.
// A repeat of the last sequence is a duplicate delivery and stays quiet.
func TestBudgetSyncHandler_LowerSequenceWarnsOncePerStream(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)

	var logs bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&logs).Level(zerolog.InfoLevel)
	t.Cleanup(func() { log.Logger = prev })
	warnings := func() int { return strings.Count(logs.String(), `"level":"warn"`) }

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	send := func(seq uint64) {
		handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
			AppUsages: map[uint32]float64{1: 1}, PeriodStart: periodStart,
			PeriodEnd: periodStart.AddDate(0, 1, 0).Add(-time.Second), ControlTimestamp: now, SequenceNumber: seq,
		}))
	}

	send(100)
	send(100)
	assert.Zero(t, warnings(), "a duplicate of the last sync is not reported")

	send(40)
	assert.Equal(t, 1, warnings())
	assert.Contains(t, logs.String(), `"received":40`)
	assert.Contains(t, logs.String(), `"last":100`)
	send(41)
	send(42)
	assert.Equal(t, 1, warnings(), "once per stream")
	assert.Equal(t, uint64(100), handler.GetLastSequenceNumber(), "lower sequences are still ignored")

	handler.StreamStarted()
	send(43)
	send(44)
	assert.Equal(t, 2, warnings(), "reported again on a new stream")

	send(101)
	assert.Equal(t, uint64(101), handler.GetLastSequenceNumber())
	assert.Equal(t, 2, warnings())
}
