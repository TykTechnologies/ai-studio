package chat_session

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyedUpstream records the X-API-Key header of each call.
func keyedUpstream(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("X-API-Key"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), keys...)
	}
}

func keyedSpec(url string) string {
	return fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"%s"}],`+
		`"components":{"securitySchemes":{"apiKey":{"type":"apiKey","in":"header","name":"X-API-Key"}}},`+
		`"security":[{"apiKey":[]}],`+
		`"paths":{"/x":{"post":{"operationId":"sendData","responses":{"200":{"description":"OK"}}}}}}`, url)
}

// A chat tool call sends the secret a $SECRET/ auth key names, not the
// reference. Window-picked tools, room default tools and tool dependencies
// are all loaded with GetToolByID, which keeps references (API responses
// show them); the call resolves them.
func TestChatToolCallResolvesSecretAuthKey(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "UPSTREAM_KEY", Value: "real-upstream-key"}))

	upstream, received := keyedUpstream(t)
	svc := services.NewService(db)
	created, err := svc.CreateTool("Keyed tool", "d", models.ToolTypeREST,
		base64.StdEncoding.EncodeToString([]byte(keyedSpec(upstream.URL))), 0, "apiKey", "$SECRET/UPSTREAM_KEY")
	require.NoError(t, err)

	loaded, err := svc.GetToolByID(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "$SECRET/UPSTREAM_KEY", loaded.AuthKey, "the getter keeps the reference")
	require.NoError(t, DecodeToolSpec(loaded))

	uid := uint(1)
	sid := "tool-secret-session"
	chatRef := &models.Chat{ID: 1, Name: "Chat", LLMSettings: &models.LLMSettings{ModelName: "dummy"},
		LLM: &models.LLM{Name: "Dummy LLM", Vendor: models.MOCK_VENDOR}}
	cs, err := NewChatSession(chatRef, ChatMessage, db, svc, nil, &uid, &sid)
	require.NoError(t, err)
	require.NoError(t, cs.initSession())
	loaded.AvailableOperations = "sendData"
	cs.tools = map[string]models.Tool{loaded.Name: *loaded}

	choice := &llms.ContentChoice{ToolCalls: []llms.ToolCall{{
		ID: "call_1", Type: "function",
		FunctionCall: &llms.FunctionCall{Name: "sendData", Arguments: `{}`},
	}}}
	cs.handleToolCalls(context.Background(), choice,
		&llms.MessageContent{Role: llms.ChatMessageTypeAI}, &llms.MessageContent{Role: llms.ChatMessageTypeTool})

	assert.Equal(t, []string{"real-upstream-key"}, received())
}
