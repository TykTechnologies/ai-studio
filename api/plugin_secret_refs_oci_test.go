package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Creating an OCI plugin with a $SECRET/ reference in its config needs
// secrets read, like any other plugin config write. The check runs before
// the image is fetched.
func TestCreateOCIPluginRequiresSecretReadForReferences(t *testing.T) {
	api, _ := setupTestAPI(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"name":"p","oci_reference":"registry.example.com/p:1.0.0","hook_type":"studio_ui","config":{"vault_token":"$SECRET/VAULT_TOKEN"}}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/plugins/oci", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) {
		return authz.NewSet(authz.Write("plugins")), nil
	}))

	api.createOCIPlugin(c)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}
