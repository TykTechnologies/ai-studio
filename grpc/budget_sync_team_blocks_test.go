package grpc

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTeamBlocks struct {
	blocks map[uint]string
	err    error
}

func (f *fakeTeamBlocks) EdgeBlocks() (map[uint]string, error) { return f.blocks, f.err }

func TestBudgetSyncService_TeamBlocks(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	var mu sync.Mutex
	var got []BudgetSyncPayload
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(event.Payload, &p))
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(got)
	}

	svc := NewBudgetSyncService(db, bus)
	src := &fakeTeamBlocks{blocks: map[uint]string{3: "team allocation exhausted"}}
	svc.SetTeamBlockSource(src)

	// Published even with no App usage, so blocks reach edges.
	svc.aggregateAndPublish()
	require.Eventually(t, func() bool { return count() == 1 }, time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.True(t, got[0].TeamBlocksIncluded)
	assert.Equal(t, "team allocation exhausted", got[0].TeamBlocks[3])
	mu.Unlock()

	// An empty set is still sent, so edges release the App.
	src.blocks = map[uint]string{}
	svc.aggregateAndPublish()
	require.Eventually(t, func() bool { return count() == 2 }, time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.True(t, got[1].TeamBlocksIncluded)
	assert.Empty(t, got[1].TeamBlocks)
	mu.Unlock()

	// A failure leaves the edges' last set in place (and, with no usage,
	// nothing is published at all).
	src.err = errors.New("db down")
	svc.aggregateAndPublish()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 2, count())
}
