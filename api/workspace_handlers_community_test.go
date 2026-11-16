//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/stretchr/testify/assert"
)

// Community Edition: every Workspaces route answers 402 enterprise_required.
func TestWorkspacesAPI_CommunityIsEnterpriseRequired(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/workspaces"},
		{"POST", "/api/v1/workspaces"},
		{"GET", "/api/v1/workspaces/1"},
		{"PATCH", "/api/v1/workspaces/1"},
		{"DELETE", "/api/v1/workspaces/1"},
		{"GET", "/api/v1/workspaces/1/objects"},
	} {
		var body interface{}
		if r.method == "POST" || r.method == "PATCH" {
			body = map[string]interface{}{"name": "W"}
		}
		w := apitest.PerformAuthRequest(api.router, r.method, r.path, body, admin.APIKey)
		assert.Equal(t, http.StatusPaymentRequired, w.Code, "%s %s: %s", r.method, r.path, w.Body.String())
		assert.Equal(t, authz.CodeEnterpriseRequired, decodeErrorCode(t, w.Body.Bytes()), "%s %s", r.method, r.path)
	}
}
