package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSelfObjectType(t *testing.T) {
	assert.True(t, IsSelfObjectType("plugin_resource:self:prompts"))
	assert.False(t, IsSelfObjectType("plugin_resource:12:prompts"))
	assert.False(t, IsSelfObjectType("llm"))
	assert.False(t, IsSelfObjectType("self:prompts"), "the placeholder only exists under the plugin_resource prefix")

	assert.Equal(t, "plugin_resource:12:prompts", ResolveSelfObjectType("plugin_resource:self:prompts", 12))
	assert.Equal(t, "plugin_resource:3:agents", ResolveSelfObjectType("plugin_resource:3:agents", 12), "explicit plugin ids are untouched")
	assert.Equal(t, "llm", ResolveSelfObjectType("llm", 12))
	assert.Equal(t, "*", ResolveSelfObjectType("*", 12))
	assert.Equal(t, "plugin_resource:12:", ResolveSelfObjectType("plugin_resource:self:", 12), "an empty slug is passed through for the caller to reject")
}

func TestManifestAppliesToTargets(t *testing.T) {
	for _, ok := range []string{"llm", "tool", "datasource", "*", "plugin_resource:1:widgets", "plugin_resource:self:widgets", "plugin_resource:self:my-type_2"} {
		assert.True(t, isValidManifestAppliesTo(ok), ok)
	}
	for _, bad := range []string{"", "app", "LLM", "plugin_resource", "plugin_resource:", "plugin_resource:self", "plugin_resource:self:",
		"plugin_resource:abc:widgets", "plugin_resource:-1:widgets", "self:widgets", "plugin_resource:1:"} {
		assert.False(t, isValidManifestAppliesTo(bad), bad)
	}

	m := baseManifest()
	m.Metadata = &ManifestMetadata{Schemas: []ManifestSchema{{Slug: "s", Name: "S", AppliesTo: []string{"plugin_resource:self:widgets"},
		Fields: []MetadataFieldDef{{Key: "k", Type: "string"}}}}}
	assert.NoError(t, m.ValidateManifest(), "self-referencing resource types are valid manifest targets")

	m.Metadata.Schemas[0].AppliesTo = []string{"llm", "apps"}
	err := m.ValidateManifest()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `applies_to entry "apps"`)
}
