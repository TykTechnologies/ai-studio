package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Filters are materialised onto the LLM on every request, so the hub's
// config JSON is parsed once per filter version and the hub's timestamps
// travel with the model, which is what the runner keys its own cache on.
func TestConvertDatabaseFilterToModel_ParsesGuardrailConfigOncePerVersion(t *testing.T) {
	parsedConfigs.Reset()

	v1 := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	row := &database.Filter{
		ID:        3,
		CreatedAt: v1.Add(-time.Hour),
		UpdatedAt: v1,
		Name:      "pii",
		Kind:      models.FilterKindGuardrail,
		Config:    `{"provider":"builtin","detectors":[{"name":"pii"}],"on_detect":"redact"}`,
	}

	first := convertDatabaseFilterToModel(row)
	require.Equal(t, models.FilterKindGuardrail, first.Kind)
	require.Equal(t, "builtin", first.Config["provider"])
	assert.Equal(t, v1, first.UpdatedAt)
	assert.Equal(t, v1.Add(-time.Hour), first.CreatedAt)

	second := convertDatabaseFilterToModel(row)
	assert.True(t, sameMap(first.Config, second.Config), "the same version is served from the parsed cache")

	// A new version from the hub is parsed afresh.
	row.UpdatedAt = v1.Add(time.Minute)
	row.Config = `{"provider":"builtin","detectors":[{"name":"secrets"}],"on_detect":"block"}`
	third := convertDatabaseFilterToModel(row)
	assert.False(t, sameMap(first.Config, third.Config))
	assert.Equal(t, "block", third.Config["on_detect"])

	// A script filter carries no config and is not cached.
	script := convertDatabaseFilterToModel(&database.Filter{ID: 4, Script: "output := {}"})
	assert.Equal(t, models.FilterKindScript, script.Kind)
	assert.Nil(t, script.Config)

	// Invalid JSON is logged and left nil, not cached as an empty config.
	bad := convertDatabaseFilterToModel(&database.Filter{ID: 5, UpdatedAt: v1, Kind: models.FilterKindGuardrail, Config: "{"})
	assert.Nil(t, bad.Config)
}

// sameMap reports whether two maps are the same underlying object.
func sameMap(a, b map[string]any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	a["__probe"] = true
	defer delete(a, "__probe")
	_, shared := b["__probe"]
	return shared
}
