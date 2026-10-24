package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// Obviously fake credentials: never real ones in tests.
const (
	redactFakeToolKey   = "fake-tool-auth-key"
	redactFakeDSConn    = "postgres://u:fake-ds-password@db/vec"
	redactFakeDSKey     = "fake-ds-conn-api-key"
	redactFakeEmbedKey  = "fake-ds-embed-key"
	redactFakeUserKey   = "fake-user-api-key"
	redactFakeUserToken = "fake-user-session-token"
)

func assertNoFakeObjectCredentials(t *testing.T, payload string) {
	t.Helper()
	for _, v := range []string{redactFakeToolKey, "fake-ds-password", redactFakeDSKey, redactFakeEmbedKey, redactFakeUserKey, redactFakeUserToken} {
		assert.NotContains(t, payload, v)
	}
}

// system.tool, system.datasource and system.user created/updated events carry
// the object with its credentials redacted (i5m), as system.llm events do.
func TestSystemEvents_ToolDatasourceUserCredentialsRedacted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)
	collector := NewTestEventCollector()
	collector.Expect(6)
	for _, topic := range []string{TopicToolCreated, TopicToolUpdated, TopicDatasourceCreated, TopicDatasourceUpdated, TopicUserCreated, TopicUserUpdated} {
		id := bus.Subscribe(topic, collector.Handler())
		defer bus.Unsubscribe(id)
	}

	tool, err := service.CreateTool("Weather", "d", models.ToolTypeREST, "", 10, "apiKey", redactFakeToolKey)
	require.NoError(t, err)
	_, err = service.UpdateTool(tool.ID, "Weather", "d2", models.ToolTypeREST, "", 10, "apiKey", models.CredentialRedacted)
	require.NoError(t, err)

	embed := EmbedderInput{Vendor: "openai", URL: "https://embed.example", APIKey: redactFakeEmbedKey, Model: "m"}
	ds, err := service.CreateDatasource("Docs", "s", "l", "", "", 10, 1, nil, redactFakeDSConn, "pgvector", redactFakeDSKey, "db", embed, true)
	require.NoError(t, err)
	_, err = service.UpdateDatasource(ds.ID, "Docs", "s2", "l", "", "", 10, redactFakeDSConn, "pgvector", models.CredentialRedacted, "db", EmbedderInput{EmbedderID: ds.EmbedderID, APIKey: RedactedEmbedderKey}, true, nil, 1)
	require.NoError(t, err)

	user, err := service.CreateUser(UserDTO{Email: "redact@example.com", Name: "Redact", Password: "password123"})
	require.NoError(t, err)
	user.APIKey = redactFakeUserKey
	user.SessionToken = redactFakeUserToken
	_, err = service.UpdateUser(user, UserDTO{Email: "redact@example.com", Name: "Redact 2"})
	require.NoError(t, err)

	require.True(t, collector.WaitWithTimeout(2*time.Second))
	require.Len(t, collector.GetEvents(), 6)
	for _, ev := range collector.GetEvents() {
		assertNoFakeObjectCredentials(t, string(ev.Payload))
		assert.Contains(t, string(ev.Payload), models.CredentialRedacted, ev.Topic)
	}
}

// Object hooks, before and after, see tool, datasource and user credentials
// redacted. A before-hook's result that leaves them redacted keeps the real
// ones.
func TestObjectHooks_ToolDatasourceUserCredentialsRedactedAndRestored(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	plugin := &hookPlugin{modify: func(obj map[string]any) {
		if _, ok := obj["description"]; ok {
			obj["description"] = "set by the hook"
		}
		if _, ok := obj["short_description"]; ok {
			obj["short_description"] = "set by the hook"
		}
	}}
	registry := NewHookRegistry()
	var regs []*pb.ObjectHookRegistration
	for _, ot := range []ObjectType{ObjectTypeTool, ObjectTypeDatasource, ObjectTypeUser} {
		regs = append(regs, &pb.ObjectHookRegistration{
			ObjectType: string(ot),
			HookTypes:  []string{string(HookBeforeCreate), string(HookAfterCreate), string(HookBeforeUpdate), string(HookAfterUpdate)},
		})
	}
	require.NoError(t, registry.RegisterHooks(1, "recorder", regs))
	manager := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{1: {ID: 1, Name: "recorder", GRPCClient: plugin, IsHealthy: true}}}
	s.HookManager = NewHookManager(registry, manager)

	tool, err := s.CreateTool("Weather", "d", models.ToolTypeREST, "", 10, "apiKey", redactFakeToolKey)
	require.NoError(t, err)
	embed := EmbedderInput{Vendor: "openai", URL: "https://embed.example", APIKey: redactFakeEmbedKey, Model: "m"}
	ds, err := s.CreateDatasource("Docs", "s", "l", "", "", 10, 1, nil, redactFakeDSConn, "pgvector", redactFakeDSKey, "db", embed, true)
	require.NoError(t, err)
	user, err := s.CreateUser(UserDTO{Email: "hook@example.com", Name: "Hook", Password: "password123"})
	require.NoError(t, err)
	user.APIKey = redactFakeUserKey
	_, err = s.UpdateUser(user, UserDTO{Email: "hook@example.com", Name: "Hook 2"})
	require.NoError(t, err)

	reqs := plugin.requests()
	require.GreaterOrEqual(t, len(reqs), 8)
	for _, req := range reqs {
		assertNoFakeObjectCredentials(t, req.ObjectJson)
	}

	storedTool, err := s.GetToolByID(tool.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", storedTool.Description, "the hook's change applies")
	assert.Equal(t, redactFakeToolKey, storedTool.AuthKey, "the redacted key in its result means unchanged")

	storedDS, err := s.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", storedDS.ShortDescription)
	assert.Equal(t, redactFakeDSConn, storedDS.DBConnString)
	assert.Equal(t, redactFakeDSKey, storedDS.DBConnAPIKey)
	assert.Equal(t, redactFakeEmbedKey, storedDS.FlattenedEmbed().APIKey)

	storedUser, err := s.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, redactFakeUserKey, storedUser.APIKey)
	assert.True(t, storedUser.DoesPasswordMatch("password123"), "the password hash is unchanged")
}

// RedactObjectCredentials keeps references, redacts a tool's dependencies,
// and leaves the original alone; RestoreObjectCredentials keeps what a hook
// set.
func TestRedactObjectCredentials_ReferencesAndDependencies(t *testing.T) {
	tool := &models.Tool{AuthKey: "$SECRET/TOOL_KEY", Dependencies: []*models.Tool{{ID: 2, AuthKey: redactFakeToolKey}}}
	got := models.RedactObjectCredentials(tool).(*models.Tool)
	assert.Equal(t, "$SECRET/TOOL_KEY", got.AuthKey)
	assert.Equal(t, models.CredentialRedacted, got.Dependencies[0].AuthKey)
	assert.Equal(t, redactFakeToolKey, tool.Dependencies[0].AuthKey, "the original is unchanged")

	b, err := json.Marshal(got)
	require.NoError(t, err)
	var modified models.Tool
	require.NoError(t, json.Unmarshal(b, &modified))
	modified.AuthKey = "set-by-hook"
	models.RestoreObjectCredentials(tool, &modified)
	assert.Equal(t, "set-by-hook", modified.AuthKey)
	assert.Equal(t, redactFakeToolKey, modified.Dependencies[0].AuthKey)

	ds := models.RedactObjectCredentials(models.Datasource{DBConnString: "$ENV/DS_CONN", DBConnAPIKey: redactFakeDSKey}).(*models.Datasource)
	assert.Equal(t, "$ENV/DS_CONN", ds.DBConnString)
	assert.Equal(t, models.CredentialRedacted, ds.DBConnAPIKey)

	u := models.RedactObjectCredentials(&models.User{Password: "hash", ResetToken: "", VerificationToken: "v"}).(*models.User)
	assert.Equal(t, models.CredentialRedacted, u.Password)
	assert.Equal(t, "", u.ResetToken, "an empty value stays empty")
	assert.Equal(t, models.CredentialRedacted, u.VerificationToken)
}
