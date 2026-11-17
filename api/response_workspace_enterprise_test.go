//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A row still stored with workspace_id 0 (written by an older build, .41)
// is reported as the Default workspace by every admin route of filters,
// embedders, secrets and datasources, as it is for the other types (.52).
func TestResponseWorkspaceID_StoredZeroIsTheDefault(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "response-workspace-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB

	filter := &models.Filter{Name: "Legacy filter", Script: []byte("filter = true")}
	embedder := &models.Embedder{Name: "Legacy embedder", Vendor: "ollama", ModelName: "nomic-embed-text"}
	secret := &secrets.Secret{VarName: "LEGACY_SECRET", Value: "v"}
	ds := &models.Datasource{Name: "Legacy datasource"}
	require.NoError(t, db.Create(filter).Error)
	require.NoError(t, db.Create(embedder).Error)
	require.NoError(t, secrets.CreateSecret(db, secret))
	require.NoError(t, db.Create(ds).Error)
	for _, m := range []struct {
		model interface{}
		id    uint
	}{{&models.Filter{}, filter.ID}, {&models.Embedder{}, embedder.ID}, {&secrets.Secret{}, secret.ID}, {&models.Datasource{}, ds.ID}} {
		require.NoError(t, db.Model(m.model).Where("id = ?", m.id).UpdateColumn("workspace_id", 0).Error)
	}

	// workspaceIDs collects every workspace_id in a response, wherever the
	// payload puts it (filters answer the object unwrapped).
	workspaceIDs := func(path string) []float64 {
		w := f.do("GET", path, nil, f.owner)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		var body interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		var out []float64
		var walk func(v interface{})
		walk = func(v interface{}) {
			switch x := v.(type) {
			case map[string]interface{}:
				for k, val := range x {
					if k == "workspace_id" {
						if n, ok := val.(float64); ok {
							out = append(out, n)
						}
						continue
					}
					walk(val)
				}
			case []interface{}:
				for _, e := range x {
					walk(e)
				}
			}
		}
		walk(body)
		return out
	}
	def := float64(f.def)
	for _, path := range []string{
		fmt.Sprintf("/api/v1/filters/%d", filter.ID),
		fmt.Sprintf("/api/v1/embedders/%d", embedder.ID),
		fmt.Sprintf("/api/v1/secrets/%d", secret.ID),
		fmt.Sprintf("/api/v1/datasources/%d", ds.ID),
	} {
		assert.Equal(t, []float64{def}, workspaceIDs(path), path)
	}
	for _, path := range []string{"/api/v1/filters", "/api/v1/embedders", "/api/v1/secrets", "/api/v1/datasources"} {
		ids := workspaceIDs(path)
		require.NotEmpty(t, ids, path)
		for _, id := range ids {
			assert.Equal(t, def, id, "%s lists the Default, never 0 or nothing", path)
		}
	}
}
