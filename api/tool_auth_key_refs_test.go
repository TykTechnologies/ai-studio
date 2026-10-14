package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Editing a tool and sending the redacted placeholder for its auth key keeps
// the stored reference, $ENV/ as well as $SECRET/. The loaded tool used to
// hold the resolved $ENV/ value, which the edit then saved in its place.
func TestToolEditWithPlaceholderKeepsAuthKeyReference(t *testing.T) {
	t.Setenv("PROBE_UPSTREAM_KEY", "plaintext-from-env")
	api, db := setupTestAPI(t)

	for _, ref := range []string{"$ENV/PROBE_UPSTREAM_KEY", "$SECRET/SOME_KEY"} {
		tool, err := api.service.CreateTool("Keyed "+ref, "d", models.ToolTypeREST, testToolSpec, 0, "apiKey", ref)
		require.NoError(t, err)

		loaded, err := api.service.GetToolByID(tool.ID)
		require.NoError(t, err)
		assert.Equal(t, ref, loaded.AuthKey, "the loaded tool keeps the reference")

		body := toolBody("Keyed "+ref+" renamed", models.ToolTypeREST, testToolSpec)
		attrs := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
		attrs["auth_key"] = services.REDACTED_VALUE
		attrs["auth_schema_name"] = "apiKey"
		w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID), body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var stored models.Tool
		require.NoError(t, db.First(&stored, tool.ID).Error)
		assert.Equal(t, ref, stored.AuthKey, "the stored reference survives the edit")
		assert.NotContains(t, w.Body.String(), "plaintext-from-env")
	}
}
