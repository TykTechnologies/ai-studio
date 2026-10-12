package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMPriceMultipliersValidate(t *testing.T) {
	assert.NoError(t, LLMPriceMultipliers(nil).Validate())
	assert.NoError(t, LLMPriceMultipliers{"batch": 0.4, "priority": 1.25, "flex": 0, "x_y-1": MaxPriceMultiplier}.Validate())
	for name, m := range map[string]LLMPriceMultipliers{
		"negative":   {"batch": -0.1},
		"too large":  {"batch": MaxPriceMultiplier + 0.01},
		"NaN":        {"batch": math.NaN()},
		"uppercase":  {"Batch": 0.5},
		"empty tier": {"": 0.5},
		"space":      {"bat ch": 0.5},
		"long tier":  {"abcdefghijklmnopqrstuvwxyz0123456": 0.5},
		"default":    {"default": 0.5},
		"auto":       {"auto": 1},
	} {
		assert.Error(t, m.Validate(), name)
	}
}

func TestLLMPriceMultipliersMultiplier(t *testing.T) {
	m := LLMPriceMultipliers{"batch": 0.4, "flex": 0}
	assert.Equal(t, 0.4, m.Multiplier("batch", 0.5), "the override wins")
	assert.Equal(t, 0.0, m.Multiplier("flex", 0.5), "a zero override is an override")
	assert.Equal(t, 1.0, m.Multiplier("priority", 1), "a tier without an entry keeps the vendor's")
	assert.Equal(t, 0.7, m.Multiplier("", 0.7), "no tier is never adjusted")
	assert.Equal(t, 0.5, LLMPriceMultipliers(nil).Multiplier("batch", 0.5))
}

// Stored as JSON, NULL when empty, and read back on both drivers' value
// types.
func TestLLMPriceMultipliersStorage(t *testing.T) {
	v, err := LLMPriceMultipliers(nil).Value()
	require.NoError(t, err)
	assert.Nil(t, v)
	v, err = LLMPriceMultipliers{}.Value()
	require.NoError(t, err)
	assert.Nil(t, v)

	db := setupTestDB(t)
	l := &LLM{Name: "A", Vendor: ANTHROPIC, PriceMultipliers: LLMPriceMultipliers{"batch": 0.4}}
	require.NoError(t, l.Create(db))
	got := getLLM(t, db, l.ID)
	assert.Equal(t, LLMPriceMultipliers{"batch": 0.4}, got.PriceMultipliers)

	plain := &LLM{Name: "B", Vendor: ANTHROPIC}
	require.NoError(t, plain.Create(db))
	var raw *string
	require.NoError(t, db.Raw("SELECT price_multipliers FROM llms WHERE id = ?", plain.ID).Scan(&raw).Error)
	assert.Nil(t, raw, "no overrides is NULL")
	assert.Nil(t, getLLM(t, db, plain.ID).PriceMultipliers)

	var m LLMPriceMultipliers
	require.NoError(t, m.Scan([]byte(`{"priority":1.5}`)))
	assert.Equal(t, LLMPriceMultipliers{"priority": 1.5}, m)
	require.NoError(t, m.Scan(nil))
	assert.Nil(t, m)
}
