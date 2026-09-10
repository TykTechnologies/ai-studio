package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginResourceType_SubmissionSchemaRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	plugin := &Plugin{Name: "schema-plugin", Command: "/usr/bin/test", HookType: HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	schema := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
	prt := &PluginResourceType{PluginID: plugin.ID, Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: schema, IsActive: true}
	require.NoError(t, prt.Create(db))

	loaded := &PluginResourceType{}
	require.NoError(t, loaded.GetByPluginAndSlug(db, plugin.ID, "agent"))
	assert.Equal(t, schema, loaded.SubmissionSchema)
	assert.True(t, json.Valid([]byte(loaded.SubmissionSchema)))
}

func TestManifestResourceType_SubmissionSchemaString(t *testing.T) {
	t.Run("inline object", func(t *testing.T) {
		var m ManifestResourceType
		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A","submission_schema":{"type":"object"}}`), &m))
		assert.JSONEq(t, `{"type":"object"}`, m.SubmissionSchemaString())
	})
	t.Run("string literal", func(t *testing.T) {
		var m ManifestResourceType
		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A","submission_schema":"{\"type\":\"object\"}"}`), &m))
		assert.JSONEq(t, `{"type":"object"}`, m.SubmissionSchemaString())
	})
	t.Run("absent or null", func(t *testing.T) {
		var m ManifestResourceType
		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A"}`), &m))
		assert.Equal(t, "", m.SubmissionSchemaString())
		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A","submission_schema":null}`), &m))
		assert.Equal(t, "", m.SubmissionSchemaString())
	})
	t.Run("malformed values are reported, not swallowed", func(t *testing.T) {
		var m ManifestResourceType
		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A","submission_schema":"{not json"}`), &m))
		_, err := m.ParseSubmissionSchema()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a JSON object")

		require.NoError(t, json.Unmarshal([]byte(`{"slug":"a","name":"A","submission_schema":["array"]}`), &m))
		_, err = m.ParseSubmissionSchema()
		require.Error(t, err)

		// ValidateManifest surfaces the same error with the resource type slug.
		pm := PluginManifest{ID: "p", Version: "1", Name: "P", Capabilities: &PluginCapabilities{Hooks: []string{"post_auth"}},
			ResourceTypes: []ManifestResourceType{m}}
		err = pm.ValidateManifest()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource type 'a'")
	})
}
