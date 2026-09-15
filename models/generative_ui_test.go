package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPresentToolSchema_Embedded(t *testing.T) {
	spec, err := PresentToolSchema()
	require.NoError(t, err)
	assert.NotEmpty(t, spec.Description)
	assert.Equal(t, "object", spec.Parameters["type"])
	props, ok := spec.Parameters["properties"].(map[string]interface{})
	require.True(t, ok)
	// The model selects a component with `component` (the library's `$type`,
	// renamed because Anthropic rejects `$` in property names); the tree
	// nests via children.
	assert.Contains(t, props, "component")
	assert.NotContains(t, props, "$type")
	assert.Contains(t, spec.Parameters, "$defs")
}

func TestClientDefinition_PresentUsesBuiltInSchema(t *testing.T) {
	tool := &Tool{ToolType: ToolTypeClient, OASSpec: `{"ui":{"kind":"present"}}`}
	def, err := tool.ClientDefinition()
	require.NoError(t, err)
	assert.Equal(t, ClientToolKindPresent, def.UI.Kind)
	assert.NotEmpty(t, def.UI.Description)
	props := def.Parameters["properties"].(map[string]interface{})
	assert.Contains(t, props, "component")

	// An explicit schema is left alone.
	custom := &Tool{ToolType: ToolTypeClient, OASSpec: `{"ui":{"kind":"present"},"parameters":{"type":"object","properties":{"x":{"type":"string"}}}}`}
	def, err = custom.ClientDefinition()
	require.NoError(t, err)
	assert.Contains(t, def.Parameters["properties"].(map[string]interface{}), "x")
}

func TestClientDefinition_DefaultsToApproval(t *testing.T) {
	tool := &Tool{ToolType: ToolTypeClient}
	def, err := tool.ClientDefinition()
	require.NoError(t, err)
	assert.Equal(t, ClientToolKindApproval, def.UI.Kind)
	assert.Equal(t, "object", def.Parameters["type"])
}
