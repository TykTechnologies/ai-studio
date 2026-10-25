package services

import (
	"encoding/base64"
	"fmt"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CallToolOperation (the embedded gateway's /tools/ endpoints, its MCP
// serving and the plugin CallToolOperation RPC) sends the secret a $SECRET/
// auth key names, and a new client once the secret changes; the tool as
// loaded keeps showing the reference.
func TestCallToolOperationResolvesSecretAuthKey(t *testing.T) {
	// The tool's API is an httptest server on 127.0.0.1.
	netguardtest.AllowInternal(t)
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	secret := &secrets.Secret{VarName: "UPSTREAM_KEY", Value: "first-key"}
	require.NoError(t, secrets.CreateSecret(db, secret))

	var mu sync.Mutex
	var keys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("X-API-Key"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"components":{"securitySchemes":{"apiKey":{"type":"apiKey","in":"header","name":"X-API-Key"}}},`+
		`"security":[{"apiKey":[]}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, upstream.URL)

	svc := NewService(db)
	tool, err := svc.CreateTool("Keyed", "d", models.ToolTypeREST, base64.StdEncoding.EncodeToString([]byte(spec)), 0, "apiKey", "$SECRET/UPSTREAM_KEY")
	require.NoError(t, err)

	_, err = svc.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	require.NoError(t, err)

	stored, err := secrets.GetSecretByVarName(db, "UPSTREAM_KEY", true)
	require.NoError(t, err)
	stored.Value = "rotated-key"
	require.NoError(t, secrets.UpdateSecret(db, stored, true))
	_, err = svc.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, []string{"first-key", "rotated-key"}, keys)
	mu.Unlock()

	loaded, err := svc.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.Equal(t, "$SECRET/UPSTREAM_KEY", loaded.AuthKey, "API responses keep the reference")

	// An $ENV/ reference the operator allows is resolved at the call too,
	// and kept when loaded.
	t.Setenv("UPSTREAM_ENV_KEY", "env-key")
	t.Setenv(secrets.EnvRefAllowlistVar, "UPSTREAM_ENV_KEY")
	envTool, err := svc.CreateTool("Env keyed", "d", models.ToolTypeREST, base64.StdEncoding.EncodeToString([]byte(spec)), 0, "apiKey", "$ENV/UPSTREAM_ENV_KEY")
	require.NoError(t, err)
	_, err = svc.CallToolOperation(envTool.ID, "sendData", nil, map[string]interface{}{}, nil)
	require.NoError(t, err)
	mu.Lock()
	assert.Equal(t, "env-key", keys[len(keys)-1])
	mu.Unlock()
	loaded, err = svc.GetToolByID(envTool.ID)
	require.NoError(t, err)
	assert.Equal(t, "$ENV/UPSTREAM_ENV_KEY", loaded.AuthKey)
}
