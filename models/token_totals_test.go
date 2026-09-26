package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTokenTotalsReadsOnlyNewRowsAfterTheFirstPass(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&LLMChatRecord{}))

	add := func(id uint, typ InteractionType, tokens int) {
		require.NoError(t, db.Create(&LLMChatRecord{ID: id, InteractionType: typ, TotalTokens: tokens}).Error)
	}
	add(1, ProxyInteraction, 10)
	add(2, ChatInteraction, 5)

	// Count the statements that read rows below an id bound, to prove later
	// reads do not rescan the table.
	var unboundedScans int
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:count_token_scans", func(tx *gorm.DB) {
		if vars := tx.Statement.Vars; len(vars) >= 1 {
			if after, ok := vars[0].(uint); ok && after == 0 {
				unboundedScans++
			}
		}
	}))

	clock := time.Unix(1_700_000_000, 0)
	tt := NewTokenTotals()
	tt.now = func() time.Time { return clock }
	read := func() (int64, map[InteractionType]int64) {
		t.Helper()
		all, byType, err := tt.Read(db)
		require.NoError(t, err)
		return all, byType
	}

	all, byType := read()
	assert.EqualValues(t, 15, all)
	assert.EqualValues(t, 10, byType[ProxyInteraction])
	assert.EqualValues(t, 5, byType[ChatInteraction])

	add(3, ProxyInteraction, 7)
	all, byType = read()
	assert.EqualValues(t, 22, all)
	assert.EqualValues(t, 17, byType[ProxyInteraction])

	// A row that commits late with a lower id than one already seen is still
	// counted when it lands inside the settle window, however many reads
	// happen in between (one collection reads three times in a second).
	add(5, ChatInteraction, 1)
	read() // sees id 5
	read()
	read()
	add(4, ChatInteraction, 2)
	clock = clock.Add(tokenTotalsSettleAfter)
	read() // settles up to id 3
	clock = clock.Add(tokenTotalsSettleAfter)
	all, byType = read() // settles up to id 5, including the late id 4
	assert.EqualValues(t, 25, all)
	assert.EqualValues(t, 8, byType[ChatInteraction])

	clock = clock.Add(tokenTotalsSettleAfter)
	all, _ = read()
	assert.EqualValues(t, 25, all, "reading again without new rows changes nothing")

	assert.Equal(t, 1, unboundedScans, "only the first read sums from id 0")
}
