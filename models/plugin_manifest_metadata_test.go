package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseManifest() *PluginManifest {
	return &PluginManifest{
		ID: "test.metadata.plugin", Version: "1.0.0", Name: "Metadata Plugin",
		Capabilities: &PluginCapabilities{Hooks: []string{"post_auth"}},
	}
}

func TestPluginManifest_MetadataSectionValidation(t *testing.T) {
	t.Run("no metadata section is fine", func(t *testing.T) {
		assert.NoError(t, baseManifest().ValidateManifest())
	})

	t.Run("valid contributions pass", func(t *testing.T) {
		m := baseManifest()
		m.Metadata = &ManifestMetadata{
			Vocabularies: []ManifestVocabulary{{Slug: "kind", Name: "Kind", Terms: []VocabularyTerm{{Value: "a"}}}},
			Schemas:      []ManifestSchema{{Slug: "s", Name: "S", AppliesTo: []string{"llm"}, Fields: []MetadataFieldDef{{Key: "kind", Type: "vocabulary", VocabularySlug: "kind"}}}},
		}
		assert.NoError(t, m.ValidateManifest())
	})

	for name, meta := range map[string]*ManifestMetadata{
		"vocabulary without slug":  {Vocabularies: []ManifestVocabulary{{Name: "x", Terms: []VocabularyTerm{{Value: "a"}}}}},
		"vocabulary without terms": {Vocabularies: []ManifestVocabulary{{Slug: "x", Name: "x"}}},
		"schema without name":      {Schemas: []ManifestSchema{{Slug: "s", AppliesTo: []string{"llm"}, Fields: []MetadataFieldDef{{Key: "k", Type: "string"}}}}},
		"schema without applies_to": {Schemas: []ManifestSchema{{Slug: "s", Name: "S", Fields: []MetadataFieldDef{{Key: "k", Type: "string"}}}}},
		"schema without fields":    {Schemas: []ManifestSchema{{Slug: "s", Name: "S", AppliesTo: []string{"llm"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			m := baseManifest()
			m.Metadata = meta
			assert.Error(t, m.ValidateManifest())
		})
	}
}

func TestPluginManifest_MetadataAndSupportsMetadataRoundTrip(t *testing.T) {
	raw := `{
	  "id": "p", "version": "1", "name": "P",
	  "capabilities": {"hooks": ["post_auth"]},
	  "resource_types": [{"slug": "widgets", "name": "Widgets", "supports_metadata": true}],
	  "metadata": {
	    "vocabularies": [{"slug": "kind", "name": "Kind", "terms": [{"value": "big", "label": "Big", "deprecated": true}]}],
	    "schemas": [{"slug": "wg", "name": "WG", "applies_to": ["plugin_resource:1:widgets"],
	      "fields": [{"key": "kind", "type": "vocabulary", "vocabulary_slug": "kind", "required": true, "gateway_visible": true, "min": 1.5}]}]
	  }
	}`
	var m PluginManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	require.NoError(t, m.ValidateManifest())
	require.Len(t, m.ResourceTypes, 1)
	assert.True(t, m.ResourceTypes[0].SupportsMetadata)
	require.NotNil(t, m.Metadata)
	assert.True(t, m.Metadata.Vocabularies[0].Terms[0].Deprecated)
	f := m.Metadata.Schemas[0].Fields[0]
	assert.True(t, f.Required)
	assert.True(t, f.GatewayVisible)
	require.NotNil(t, f.Min)
	assert.Equal(t, 1.5, *f.Min)
	assert.Nil(t, f.Max)
}
