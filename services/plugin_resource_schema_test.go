package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSubmissionSchema(t *testing.T) {
	assert.NoError(t, ValidateSubmissionSchema(""))
	assert.NoError(t, ValidateSubmissionSchema("   "))
	assert.NoError(t, ValidateSubmissionSchema(`{"type":"object","properties":{"name":{"type":"string"}}}`))
	assert.NoError(t, ValidateSubmissionSchema(`{"properties":{"name":{"type":"string"}}}`), "type may be omitted")

	err := ValidateSubmissionSchema(`{"type":"array"}`)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSubmissionSchema)

	err = ValidateSubmissionSchema(`not json`)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSubmissionSchema)

	err = ValidateSubmissionSchema(`{"type":"object","properties":{"x":{"type":"no-such-type"}}}`)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSubmissionSchema)
}

func TestValidatePayloadAgainstSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"name":{"type":"string","minLength":1},"risk":{"type":"string","enum":["low","high"]}},"required":["name"]}`

	assert.NoError(t, ValidatePayloadAgainstSchema("", map[string]interface{}{"anything": true}))
	assert.NoError(t, ValidatePayloadAgainstSchema(schema, map[string]interface{}{"name": "Triage", "risk": "low"}))

	err := ValidatePayloadAgainstSchema(schema, map[string]interface{}{"risk": "medium"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
	assert.Contains(t, err.Error(), "risk")

	err = ValidatePayloadAgainstSchema(schema, nil)
	require.Error(t, err)
}

func TestRegisterPluginResourceTypes_SubmissionSchema(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "schema-plugin")

	schema := `{"type":"object","required":["name"]}`
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: schema},
	}))

	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)
	assert.Equal(t, schema, prt.SubmissionSchema)

	// Update replaces the schema
	updated := `{"type":"object","required":["name","purpose"]}`
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: updated},
	}))
	prt, err = service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)
	assert.Equal(t, updated, prt.SubmissionSchema)

	// Invalid schema is rejected before any write
	err = service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "prompt", Name: "Prompt", SubmissionSchema: `{"type":"string"}`},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSubmissionSchema)
	_, err = service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "prompt")
	assert.Error(t, err, "nothing written for the rejected batch")
}

func TestDeactivatePluginResourceTypesExcept(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "deactivate-plugin")
	other := createTestPlugin(t, db, "other-plugin")

	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent"}, {Slug: "prompt", Name: "Prompt"}, {Slug: "skill", Name: "Skill"},
	}))
	require.NoError(t, service.RegisterPluginResourceTypes(other.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Other Agent"},
	}))

	n, err := service.DeactivatePluginResourceTypesExcept(plugin.ID, []string{"agent"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	for slug, wantActive := range map[string]bool{"agent": true, "prompt": false, "skill": false} {
		prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, slug)
		require.NoError(t, err)
		assert.Equal(t, wantActive, prt.IsActive, slug)
	}
	otherAgent, err := service.GetPluginResourceTypeByPluginAndSlug(other.ID, "agent")
	require.NoError(t, err)
	assert.True(t, otherAgent.IsActive)

	// Empty keep list deactivates everything that is still active
	n, err = service.DeactivatePluginResourceTypesExcept(plugin.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Re-registering reactivates
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{Slug: "prompt", Name: "Prompt"}}))
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "prompt")
	require.NoError(t, err)
	assert.True(t, prt.IsActive)
}
