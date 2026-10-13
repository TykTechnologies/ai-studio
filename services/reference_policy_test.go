package services

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The master key (or any variable the operator has not listed) never leaves
// AI Studio through a reference a resource writer chose: not in a tool call
// to the writer's server, not in the LLM settings the gateway sends with,
// and not in what the API returns. A listed name resolves.
func TestUnlistedEnvReferencesNeverResolve(t *testing.T) {
	const canary = "canary-master-key-value"
	t.Setenv("TYK_AI_SECRET_KEY", canary)
	t.Setenv("UPSTREAM_REGION", "eu-west-1")
	t.Setenv(secrets.EnvRefAllowlistVar, "UPSTREAM_REGION")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	svc := NewService(db)

	var mu sync.Mutex
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-API-Key"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"components":{"securitySchemes":{"apiKey":{"type":"apiKey","in":"header","name":"X-API-Key"}}},`+
		`"security":[{"apiKey":[]}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, upstream.URL)

	tool, err := svc.CreateTool("Probe", "d", models.ToolTypeREST, base64.StdEncoding.EncodeToString([]byte(spec)), 0, "apiKey", "$ENV/TYK_AI_SECRET_KEY")
	require.NoError(t, err)
	_, err = svc.CallToolOperation(tool.ID, "sendData", nil, map[string]interface{}{}, nil)
	require.NoError(t, err)
	mu.Lock()
	for _, h := range seen {
		assert.NotContains(t, h, canary, "the tool call sent the master key")
	}
	mu.Unlock()

	llm := &models.LLM{Name: "Probe LLM", Vendor: models.OPENAI, APIKey: "$ENV/TYK_AI_SECRET_KEY",
		APIEndpoint: "https://api.openai.com/v1", Metadata: models.JSONMap{"region": "$ENV/UPSTREAM_REGION", "k": "$ENV/TYK_AI_SECRET_KEY"}}
	require.NoError(t, db.Create(llm).Error)

	// The gateway's resolved view.
	resolved, err := svc.GetLLMByName("Probe LLM")
	require.NoError(t, err)
	assert.Equal(t, "$ENV/TYK_AI_SECRET_KEY", resolved.APIKey)

	// The API's view keeps every reference, the listed one included.
	shown, err := svc.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "$ENV/TYK_AI_SECRET_KEY", shown.APIKey)
	assert.Equal(t, "$ENV/UPSTREAM_REGION", shown.Metadata["region"])
	for _, v := range []string{shown.APIKey, fmt.Sprint(shown.Metadata)} {
		assert.False(t, strings.Contains(v, canary))
	}

	// A listed name resolves where the value is used.
	assert.Equal(t, "eu-west-1", secrets.ResolveRef("$ENV/UPSTREAM_REGION"))
}
