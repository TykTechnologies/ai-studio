package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Adding a $SECRET/ reference to plugin config needs secrets read: AI Studio
// hands the resolved value to the plugin.
func TestRequireSecretReadForReferences(t *testing.T) {
	run := func(perms authz.Set, old, updated map[string]interface{}) (bool, int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return perms, nil }))
		ok := requireSecretReadForReferences(c, old, updated)
		return ok, w.Code
	}
	pluginEditor := authz.NewSet(authz.Write("plugins"))
	secretReader := authz.NewSet(authz.Write("plugins"), authz.Read("secrets"))
	withRef := map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN"}

	ok, code := run(pluginEditor, nil, withRef)
	assert.False(t, ok, "a new reference without secrets read")
	assert.Equal(t, http.StatusForbidden, code)

	ok, _ = run(secretReader, nil, withRef)
	assert.True(t, ok, "a new reference with secrets read")

	ok, _ = run(pluginEditor, withRef, map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN", "x": "y"})
	assert.True(t, ok, "keeping an existing reference needs nothing extra")

	ok, _ = run(pluginEditor, nil, map[string]interface{}{"vault_token": "literal"})
	assert.True(t, ok, "no reference, nothing to check")
}
