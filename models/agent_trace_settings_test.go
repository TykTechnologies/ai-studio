package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppTracingSettingsValidate(t *testing.T) {
	days := func(n int) *int { return &n }
	for _, ok := range []AppTracingSettings{
		{},
		{ContentMode: "full", MaxRequestOverride: "off", SessionIdleTimeout: "2h", PayloadDays: days(0)},
		{SessionIdleTimeout: "1m"},
		{SessionIdleTimeout: "168h", PayloadDays: days(MaxTracingPayloadDays)},
	} {
		assert.NoError(t, ok.Validate(), "%+v", ok)
	}
	for _, bad := range []AppTracingSettings{
		{ContentMode: "FULL"},
		{MaxRequestOverride: "everything"},
		{SessionIdleTimeout: "2 hours"},
		{SessionIdleTimeout: "59s"},
		{SessionIdleTimeout: "169h"},
		{PayloadDays: days(-1)},
		{PayloadDays: days(MaxTracingPayloadDays + 1)},
	} {
		assert.Error(t, bad.Validate(), "%+v", bad)
	}
}

// Edges get every setting but the hub-only Portal content grant; settings
// that all inherit are stored as NULL and sent as nothing.
func TestAppTracingSettingsStorageAndEdgeForm(t *testing.T) {
	v, err := AppTracingSettings{}.Value()
	require.NoError(t, err)
	assert.Nil(t, v)
	assert.Empty(t, AppTracingSettings{}.EdgeJSON())
	assert.Empty(t, AppTracingSettings{OwnerMayViewContent: true}.EdgeJSON(), "the grant alone is nothing for edges")

	in := AppTracingSettings{ContentMode: "redacted", OwnerMayViewContent: true}
	assert.JSONEq(t, `{"content_mode":"redacted"}`, in.EdgeJSON())

	v, err = in.Value()
	require.NoError(t, err)
	var out AppTracingSettings
	require.NoError(t, out.Scan(v))
	assert.Equal(t, in, out, "the hub stores the grant")
	require.NoError(t, out.Scan(nil))
	assert.Equal(t, AppTracingSettings{}, out)
}
