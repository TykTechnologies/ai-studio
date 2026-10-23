package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Obviously fake credentials: never real ones in tests.
const (
	redactFakeGuardrailKey = "fake-guardrail-api-key"
	redactFakeLLMKey       = "fake-llm-api-key"
)

func redactGuardrailSpec(apiKey string) FilterSpec {
	return FilterSpec{Name: "Classifier", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":   guardrails.ProviderHTTP,
		"detectors":  []any{map[string]any{"name": "toxicity"}},
		"on_detect":  "block",
		"connection": map[string]any{"endpoint": "https://classifier.example/v1", "api_key": apiKey},
	}}
}

// 2ze: system.filter events carry a guardrail's connection with its literal
// secrets redacted; references and non-secret fields are kept.
func TestSystemEvents_FilterConnectionSecretsRedacted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)
	collector := NewTestEventCollector()
	collector.Expect(2)
	for _, topic := range []string{TopicFilterCreated, TopicFilterUpdated} {
		id := bus.Subscribe(topic, collector.Handler())
		defer bus.Unsubscribe(id)
	}

	f, err := service.CreateFilterFromSpec(redactGuardrailSpec(redactFakeGuardrailKey))
	require.NoError(t, err)
	_, err = service.UpdateFilterFromSpec(f.ID, redactGuardrailSpec(redactFakeGuardrailKey))
	require.NoError(t, err)

	require.True(t, collector.WaitWithTimeout(2*time.Second))
	for _, ev := range collector.GetEvents() {
		assert.NotContains(t, string(ev.Payload), redactFakeGuardrailKey, ev.Topic)
		assert.Contains(t, string(ev.Payload), models.CredentialRedacted, ev.Topic)
		assert.Contains(t, string(ev.Payload), "https://classifier.example/v1", "the endpoint is not a secret")
	}
}

// 2ze: a tool's and an LLM's filters are redacted with them (tool events
// and hooks, LLM events and hooks), and a before-hook's result that still
// reads [redacted] gets the connection secret back.
func TestRedactObjectCredentials_NestedFilters(t *testing.T) {
	filter := models.Filter{ID: 3, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":   guardrails.ProviderHTTP,
		"connection": map[string]any{"endpoint": "https://classifier.example/v1", "api_key": redactFakeGuardrailKey},
	}}
	ref := models.Filter{ID: 4, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": guardrails.ProviderHTTP, "connection": map[string]any{"api_key": "$SECRET/CLASSIFIER_KEY"},
	}}
	tool := &models.Tool{Filters: []models.Filter{filter, ref}}
	llm := &models.LLM{Filters: []*models.Filter{&filter}}

	for _, obj := range []any{tool, llm} {
		b, err := json.Marshal(models.RedactObjectCredentials(obj))
		require.NoError(t, err)
		assert.NotContains(t, string(b), redactFakeGuardrailKey)
		assert.Contains(t, string(b), "https://classifier.example/v1")
	}
	b, _ := json.Marshal(models.RedactObjectCredentials(tool))
	assert.Contains(t, string(b), "$SECRET/CLASSIFIER_KEY", "a reference is kept")
	assert.Equal(t, redactFakeGuardrailKey, tool.Filters[0].Config["connection"].(map[string]any)["api_key"], "the original is unchanged")

	var modified models.Tool
	require.NoError(t, json.Unmarshal(b, &modified))
	models.RestoreObjectCredentials(tool, &modified)
	assert.Equal(t, redactFakeGuardrailKey, modified.Filters[0].Config["connection"].(map[string]any)["api_key"])
}

// aia: system.app events carry the App without its gateway key or the
// credentials of the LLMs, tools and datasources it preloads; system.group
// events carry their members without credentials.
func TestSystemEvents_AppAndGroupCredentialsRedacted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)
	collector := NewTestEventCollector()
	collector.Expect(4)
	for _, topic := range []string{TopicAppCreated, TopicAppUpdated, TopicGroupCreated, TopicGroupUpdated} {
		id := bus.Subscribe(topic, collector.Handler())
		defer bus.Unsubscribe(id)
	}

	user, err := service.CreateUser(UserDTO{Email: "owner@example.com", Name: "Owner", Password: "password123"})
	require.NoError(t, err)
	llm, err := service.CreateLLM("Keyed", redactFakeLLMKey, "", 5, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil, false, nil, nil)
	require.NoError(t, err)
	tool, err := service.CreateTool("Weather", "d", models.ToolTypeREST, "", 1, "apiKey", redactFakeToolKey)
	require.NoError(t, err)
	// Reachable through the gateway, so an App may be granted it.
	require.NoError(t, service.DB.Model(tool).Updates(map[string]any{"rest_access_disabled": false, "mcp_access_disabled": false}).Error)
	app, err := service.CreateApp("Keyed App", "", user.ID, nil, []uint{llm.ID}, []uint{tool.ID}, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.ActivateAppCredential(app.ID))
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotEmpty(t, app.Credential.Secret)
	_, err = service.UpdateApp(app.ID, "Keyed App 2", "", user.ID, nil, []uint{llm.ID}, []uint{tool.ID}, nil, nil, nil)
	require.NoError(t, err)

	member, err := service.CreateUser(UserDTO{Email: "member@example.com", Name: "Member", Password: "password123"})
	require.NoError(t, err)
	member.APIKey = redactFakeUserKey
	require.NoError(t, service.DB.Save(member).Error)
	group, err := service.CreateGroup("Team", []uint{member.ID}, nil, nil, nil)
	require.NoError(t, err)
	_, err = service.UpdateGroup(group.ID, "Team 2", []uint{member.ID}, nil, nil, nil)
	require.NoError(t, err)

	require.True(t, collector.WaitWithTimeout(2*time.Second))
	for _, ev := range collector.GetEvents() {
		payload := string(ev.Payload)
		for _, secret := range []string{app.Credential.Secret, redactFakeLLMKey, redactFakeToolKey, redactFakeUserKey} {
			assert.NotContains(t, payload, secret, ev.Topic)
		}
	}
}
