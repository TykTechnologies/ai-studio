package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// midsommar-clean-tgoe: chat users are not administrators, and nothing the
// chat surfaces send them may carry a tool's or datasource's credentials.

const (
	tgoeToolKey   = "tool-auth-key-tgoe"
	tgoeConnPass  = "conn-password-tgoe"
	tgoeVectorKey = "resolved-vector-key-tgoe"
	tgoeSpec      = "openapi-spec-tgoe"
)

// The session event the chat stream opens with: a datasource as the chat
// session holds it (the $SECRET/ vector key already resolved by
// AddDatasource, an inline password in the connection string) and a tool
// with an auth key. Only ids, names and descriptions go out.
func TestChatSessionEvent_SendsNoCredentials(t *testing.T) {
	tools := map[string]models.Tool{
		"weather": {ID: 7, Name: "Weather", Description: "forecasts", ToolType: "REST", AuthKey: tgoeToolKey, AuthSchemaName: "apiKey", OASSpec: tgoeSpec},
	}
	datasources := []*models.Datasource{
		{ID: 9, Name: "Docs", ShortDescription: "the manual", DBConnString: "postgres://u:" + tgoeConnPass + "@db/docs", DBConnAPIKey: tgoeVectorKey},
	}

	raw, err := json.Marshal(sessionEventMessage("sess-1", tools, datasources))
	require.NoError(t, err)
	body := string(raw)
	for _, secret := range []string{tgoeToolKey, tgoeConnPass, tgoeVectorKey, tgoeSpec, "db_conn_string", "db_conn_api_key", "auth_key", "oas_spec"} {
		assert.NotContains(t, body, secret)
	}

	// What the chat pages read is still there: the ids (to mark selected
	// tools and datasources) and the names.
	var msg struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
		Tools   []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"tools"`
		Datasources []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"datasources"`
	}
	require.NoError(t, json.Unmarshal(raw, &msg))
	assert.Equal(t, eventSession, msg.Type)
	assert.Equal(t, "sess-1", msg.Payload)
	require.Len(t, msg.Tools, 1)
	assert.Equal(t, uint(7), msg.Tools[0].ID)
	assert.Equal(t, "Weather", msg.Tools[0].Name)
	require.Len(t, msg.Datasources, 1)
	assert.Equal(t, uint(9), msg.Datasources[0].ID)
	assert.Equal(t, "Docs", msg.Datasources[0].Name)
}

// The portal's agent list and detail (/common/agents) name the agent and
// nothing it is built from: no agent configuration, plugin configuration or
// App, which can carry credentials. The ids stay.
func TestCommonAgents_SendNoConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	r := NewAPI(service, true, authService, cfg, nil, emptyFile, nil).router

	chatUser := models.NewUser()
	chatUser.Email, chatUser.Name, chatUser.Password, chatUser.EmailVerified, chatUser.ShowChat = "chatter@tgoe.test", "Chatter", "hash", true, true
	require.NoError(t, chatUser.Create(db))

	plugin := &models.Plugin{Name: "agent-plugin-tgoe", Command: "file:///bin/true", HookType: models.HookTypeAgent, IsActive: true,
		Config: map[string]interface{}{"plugin_token": "plugin-secret-tgoe"}}
	require.NoError(t, db.Create(plugin).Error)
	cred := &models.Credential{KeyID: "key-tgoe", Secret: "cred-secret-tgoe", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "app-tgoe", UserID: chatUser.ID, CredentialID: cred.ID, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	ag := models.NewAgentConfig()
	ag.Name, ag.Slug, ag.PluginID, ag.AppID = "Helper", "helper-tgoe", plugin.ID, app.ID
	ag.Config = map[string]interface{}{"api_key": "agent-secret-tgoe"}
	require.NoError(t, db.Create(ag).Error) // no teams: public

	for _, path := range []string{"/common/agents", fmt.Sprintf("/common/agents/%d", ag.ID)} {
		w := apitest.PerformAuthRequest(r, http.MethodGet, path, nil, chatUser.APIKey)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		body := w.Body.String()
		for _, secret := range []string{"agent-secret-tgoe", "plugin-secret-tgoe", "cred-secret-tgoe", "key-tgoe", `"config":{`, `"app":{`, `"plugin":{`} {
			assert.NotContains(t, body, secret, path)
		}
		assert.Contains(t, body, "Helper", path)
		assert.Contains(t, body, fmt.Sprintf(`"app_id":%d`, app.ID), path)
		assert.Contains(t, body, fmt.Sprintf(`"plugin_id":%d`, plugin.ID), path)
	}
}
