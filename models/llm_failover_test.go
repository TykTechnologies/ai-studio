package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLLMFailover_RoundTripsThroughTheDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&LLM{}))

	on := true
	primary := &LLM{
		Name:   "Primary",
		Vendor: OPENAI,
		Active: true,
		Failover: LLMFailover{
			Targets:  []LLMFailoverTarget{{LLMID: 2, Model: "claude-sonnet-4"}, {LLMID: 3, Model: "gpt-4o-mini"}},
			Triggers: &LLMFailoverTriggers{StatusCodes: []int{503}, OnTimeout: &on},
		},
	}
	require.NoError(t, db.Create(primary).Error)
	require.NoError(t, db.Create(&LLM{Name: "Plain", Vendor: OPENAI, Active: true}).Error)

	var loaded LLM
	require.NoError(t, db.First(&loaded, primary.ID).Error)
	assert.Equal(t, primary.Failover, loaded.Failover)
	assert.True(t, loaded.Failover.Enabled())

	// An LLM without a waterfall must read back as empty, and must be stored
	// as NULL so pre-existing rows and new rows are indistinguishable.
	var plain LLM
	require.NoError(t, db.Where("name = ?", "Plain").First(&plain).Error)
	assert.False(t, plain.Failover.Enabled())

	var nullCount int64
	require.NoError(t, db.Model(&LLM{}).Where("failover IS NULL").Count(&nullCount).Error)
	assert.Equal(t, int64(1), nullCount, "the LLM without targets is stored as NULL")

	// Clearing the waterfall writes NULL again rather than "{}".
	loaded.Failover = LLMFailover{}
	require.NoError(t, db.Save(&loaded).Error)
	require.NoError(t, db.Model(&LLM{}).Where("failover IS NULL").Count(&nullCount).Error)
	assert.Equal(t, int64(2), nullCount)
}

func TestLLMFailover_EffectiveTriggersDefaults(t *testing.T) {
	got := LLMFailover{}.EffectiveTriggers()
	assert.True(t, got.OnTimeout)
	assert.True(t, got.OnConnectionError)
	assert.Equal(t, 0, got.AttemptTimeoutSecond)
	for _, code := range DefaultFailoverStatusCodes {
		assert.True(t, got.StatusCodes[code], "default code %d", code)
	}
	assert.False(t, got.StatusCodes[403])
}

func TestLLMFailover_EffectiveTriggersOverridesAndSanitises(t *testing.T) {
	off := false
	f := LLMFailover{Triggers: &LLMFailoverTriggers{
		StatusCodes:          []int{503, 403, 429, 200},
		OnTimeout:            &off,
		AttemptTimeoutSecond: 15,
	}}
	got := f.EffectiveTriggers()
	assert.False(t, got.OnTimeout)
	assert.True(t, got.OnConnectionError, "unset pointer keeps the default")
	assert.Equal(t, 15, got.AttemptTimeoutSecond)
	assert.Equal(t, map[int]bool{503: true, 429: true}, got.StatusCodes,
		"403 and 200 are dropped even when a stored row lists them")
}

func TestFailoverStatusCodeAllowed(t *testing.T) {
	for _, code := range []int{408, 429, 500, 502, 503, 504, 599} {
		assert.True(t, FailoverStatusCodeAllowed(code), "%d", code)
	}
	for _, code := range []int{200, 400, 401, 403, 404, 413, 422, 600} {
		assert.False(t, FailoverStatusCodeAllowed(code), "%d", code)
	}
}
