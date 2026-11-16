//go:build enterprise
// +build enterprise

package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Secret names (midsommar-clean-oztk.47): a $SECRET/NAME reference resolves
// by name, so a name keeps meaning one secret in the referencing object's
// Workspace.

func secretAttrs(name string, extra map[string]any) map[string]any {
	a := map[string]any{"var_name": name, "value": "v"}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

// A referenced secret is neither deleted (single or bulk) nor renamed.
func TestSecretNames_ReferencedSecretIsKept(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "secret-names-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB
	used, _ := f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("USED", nil)), f.inW1))
	f.created(t, f.do("POST", "/api/v1/llms", body(llmAttrs("User", map[string]any{"api_key": "$SECRET/USED"})), f.inW1))

	assert.Equal(t, http.StatusConflict, f.do("DELETE", fmt.Sprintf("/api/v1/secrets/%d", used), nil, f.inW1).Code)
	assert.Equal(t, http.StatusConflict, f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", used), body(map[string]any{"var_name": "RENAMED"}), f.inW1).Code)
	w := f.do("POST", "/api/v1/secrets/bulk", map[string]any{"action": "delete", "ids": []uint{used}}, f.owner) // bulk routes are global-only
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "referenced")
	var live int64
	require.NoError(t, db.Model(&secrets.Secret{}).Where("id = ? AND var_name = ?", used, "USED").Count(&live).Error)
	assert.EqualValues(t, 1, live, "kept, under its name")

	// A connection string is a holder too (not recorded, scanned).
	conn, _ := f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("CONN", nil)), f.inW1))
	require.NoError(t, db.Create(&models.Datasource{Name: "Store", DBConnString: "$SECRET/CONN", WorkspaceID: f.w1}).Error)
	assert.Equal(t, http.StatusConflict, f.do("DELETE", fmt.Sprintf("/api/v1/secrets/%d", conn), nil, f.inW1).Code)

	// Unreferenced: delete and rename work.
	free, _ := f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("FREE", nil)), f.inW1))
	assert.Equal(t, http.StatusOK, f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", free), body(map[string]any{"var_name": "FREE2"}), f.inW1).Code)
	assert.Equal(t, http.StatusNoContent, f.do("DELETE", fmt.Sprintf("/api/v1/secrets/%d", free), nil, f.inW1).Code)
}

// A name is unique among live secrets across Workspaces (exact match), on
// create and on rename.
func TestSecretNames_UniqueAcrossWorkspaces(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "secret-names-test-key")
	f := setupWSB4(t)
	f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("SHARED", nil)), f.inW1))
	assert.Equal(t, http.StatusConflict, f.do("POST", "/api/v1/secrets", body(secretAttrs("SHARED", map[string]any{"workspace_id": f.w2})), f.owner).Code)
	assert.Equal(t, http.StatusConflict, f.do("POST", "/api/v1/secrets", body(secretAttrs("SHARED", nil)), f.inW1).Code)
	other, _ := f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("OTHER", map[string]any{"workspace_id": f.w2})), f.owner))
	assert.Equal(t, http.StatusConflict, f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", other), body(map[string]any{"var_name": "SHARED"}), f.owner).Code)
	// References match exactly, so does uniqueness.
	f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("shared", map[string]any{"workspace_id": f.w2})), f.owner))
	// Saving a secret under its own name is not a clash.
	assert.Equal(t, http.StatusOK, f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", other), body(map[string]any{"var_name": "OTHER", "value": "new"}), f.owner).Code)
}

// The attack of .47: a W1 object keeps a reference to a name no secret has
// (deleted before .47, or written by bulk SQL). A secret taking that name
// in another Workspace would be resolved for the W1 object, so it is
// refused; in W1 itself the name may be taken. Each kind of holder counts:
// a recorded LLM key, a datasource connection string, a guardrail filter's
// connection.
func TestSecretNames_DanglingReferenceBlocksOtherWorkspaces(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "secret-names-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB
	require.NoError(t, db.Create(&models.LLM{Name: "Dangling", Vendor: models.OPENAI, APIKey: "$SECRET/PROD_OPENAI_KEY",
		APIEndpoint: "https://attacker.example", WorkspaceID: f.w1}).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Store", DBConnString: "$SECRET/DS_CONN", WorkspaceID: f.w1}).Error)
	require.NoError(t, db.Create(&models.Filter{Name: "Guard", Kind: models.FilterKindGuardrail, WorkspaceID: f.w1,
		Config: models.JSONMap{"provider": "http", "connection": map[string]interface{}{"api_key": "$SECRET/GUARD_KEY"}}}).Error)

	for _, name := range []string{"PROD_OPENAI_KEY", "DS_CONN", "GUARD_KEY"} {
		w := f.do("POST", "/api/v1/secrets", body(secretAttrs(name, map[string]any{"workspace_id": f.w2})), f.owner)
		assert.Equal(t, http.StatusConflict, w.Code, "%s: %s", name, w.Body.String())
		// Renaming a W2 secret onto the name is the same.
		id, _ := f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("W2_"+name, map[string]any{"workspace_id": f.w2})), f.owner))
		assert.Equal(t, http.StatusConflict, f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", id), body(map[string]any{"var_name": name}), f.owner).Code, name)
	}
	var n int64
	require.NoError(t, db.Model(&secrets.Secret{}).Where("var_name = ?", "PROD_OPENAI_KEY").Count(&n).Error)
	assert.Zero(t, n, "no W2 secret answers the W1 LLM's reference")
	_, err := secrets.GetSecretByVarName(db, "PROD_OPENAI_KEY", true)
	assert.Error(t, err, "the W1 LLM cannot resolve a W2 secret")

	// In W1 the dangling reference may be filled.
	f.created(t, f.do("POST", "/api/v1/secrets", body(secretAttrs("PROD_OPENAI_KEY", nil)), f.inW1))
}

// Duplicates from before .47: one Workspace resolves to the lowest id
// (0 read as the Default); across Workspaces the name resolves to nothing,
// cannot be referenced, and neither duplicate moves.
func TestSecretNames_GrandfatheredDuplicates(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "secret-names-test-key")
	f := setupWSB4(t)
	db := f.api.service.DB
	mk := func(name string, ws uint) uint {
		s := &secrets.Secret{VarName: name, Value: "v", WorkspaceID: ws}
		require.NoError(t, secrets.CreateSecret(db, s)) // the model call: no name check, as before .47
		return s.ID
	}

	first := mk("SAME_WS", f.w1)
	mk("SAME_WS", f.w1)
	got, err := secrets.FindSecretByVarName(db, "SAME_WS")
	require.NoError(t, err)
	assert.Equal(t, first, got.ID, "lowest id")

	legacy := mk("DEFAULTS", 0)
	mk("DEFAULTS", f.def)
	got, err = secrets.FindSecretByVarName(db, "DEFAULTS")
	require.NoError(t, err, "0 is the Default")
	assert.Equal(t, legacy, got.ID)

	split := mk("SPLIT", f.w1)
	mk("SPLIT", f.w2)
	_, err = secrets.FindSecretByVarName(db, "SPLIT")
	assert.ErrorIs(t, err, secrets.ErrSecretNameAmbiguous)
	_, err = secrets.GetSecretByVarName(db, "SPLIT", false)
	assert.ErrorIs(t, err, secrets.ErrSecretNameAmbiguous)
	assertCode(t, f.do("POST", "/api/v1/llms", body(llmAttrs("Split", map[string]any{"api_key": "$SECRET/SPLIT"})), f.inW1),
		http.StatusForbidden, authz.CodePermissionDenied)

	w := f.do("PATCH", fmt.Sprintf("/api/v1/secrets/%d", split), body(map[string]any{"workspace_id": f.def}), f.owner)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	ws, _ := f.stored(t, &secrets.Secret{}, split)
	assert.Equal(t, f.w1, ws)
}
