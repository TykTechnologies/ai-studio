package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Edges decide what to capture from the agent tracing settings, so the App
// overrides and the per-resource caps travel with the config, and a change
// marks edges pending. owner_may_view_content is hub-only and never sent.
func TestSnapshot_AgentTracingSettings(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	tool := &models.Tool{Name: "Tracing Tool", ToolType: models.ToolTypeREST, Active: true}
	require.NoError(t, db.Create(tool).Error)
	ds := &models.Datasource{Name: "Tracing DS", Active: true}
	require.NoError(t, db.Create(ds).Error)

	before, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	days := 3
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", llms[1].ID).
		Updates(map[string]any{"tracing_content_mode_max": "metadata", "tracing_propagation_opt_out": true}).Error)
	require.NoError(t, db.Model(&models.App{}).Where("id = ?", apps[0].ID).
		Update("tracing", models.AppTracingSettings{ContentMode: "redacted", PayloadDays: &days, OwnerMayViewContent: true}).Error)
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", tool.ID).Update("tracing_content_mode_max", "hashed").Error)
	require.NoError(t, db.Model(&models.Datasource{}).Where("id = ?", ds.ID).Update("tracing_content_mode_max", "off").Error)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.NotEqual(t, before.Checksum, snap.Checksum, "a tracing change must mark edges pending")

	for _, l := range snap.Llms {
		if l.Id == uint32(llms[1].ID) {
			assert.Equal(t, "metadata", l.TracingContentModeMax)
			assert.True(t, l.TracingPropagationOptOut)
		} else {
			assert.Empty(t, l.TracingContentModeMax)
			assert.False(t, l.TracingPropagationOptOut, "propagation stays on by default")
		}
	}

	byApp := map[uint32]string{}
	for _, a := range snap.Apps {
		byApp[a.Id] = a.Tracing
	}
	assert.Empty(t, byApp[uint32(apps[1].ID)], "an App that inherits everything sends nothing")
	require.NotEmpty(t, byApp[uint32(apps[0].ID)])
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(byApp[uint32(apps[0].ID)]), &sent))
	assert.Equal(t, "redacted", sent["content_mode"])
	assert.EqualValues(t, 3, sent["payload_days"])
	assert.NotContains(t, sent, "owner_may_view_content", "the Portal content grant is hub-only")

	var toolCap, dsCap string
	for _, tl := range snap.Tools {
		if tl.Id == uint32(tool.ID) {
			toolCap = tl.TracingContentModeMax
		}
	}
	for _, d := range snap.Datasources {
		if d.Id == uint32(ds.ID) {
			dsCap = d.TracingContentModeMax
		}
	}
	assert.Equal(t, "hashed", toolCap)
	assert.Equal(t, "off", dsCap)

	// The pull-on-miss App config carries the same settings.
	var app models.App
	require.NoError(t, db.Preload("LLMs").First(&app, apps[0].ID).Error)
	assert.Equal(t, byApp[uint32(apps[0].ID)], server.convertAppToProto(&app).Tracing)
}
