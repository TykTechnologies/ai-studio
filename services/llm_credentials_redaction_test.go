package services

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// Obviously fake credentials: never real ones in tests.
const (
	redactFakeKey    = "sk-fake-redaction-key"
	redactFakeAccess = "AKIAFAKEREDACTION000"
	redactFakeSecret = "fake/redaction+secret"
)

func redactionTestMetadata() models.JSONMap {
	return models.JSONMap{
		"aws_access_key_id":     redactFakeAccess,
		"aws_secret_access_key": redactFakeSecret,
		"aws_session_token":     "$SECRET/AWS_SESSION",
		"aws_region":            "us-east-1",
	}
}

func assertNoFakeCredentials(t *testing.T, payload string) {
	t.Helper()
	for _, v := range []string{redactFakeKey, redactFakeAccess, redactFakeSecret} {
		assert.NotContains(t, payload, v)
	}
}

// system.llm.created and system.llm.updated carry the LLM with its
// credentials redacted and references kept (aa5). That payload is what every
// subscribed plugin receives, and what the cluster relay writes, as is, to
// cluster_events (pkg/cluster/relay.go: every system.* event is relayed).
func TestSystemEvents_LLMCredentialsRedacted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)
	collector := NewTestEventCollector()
	collector.Expect(2)
	created := bus.Subscribe(TopicLLMCreated, collector.Handler())
	defer bus.Unsubscribe(created)
	updated := bus.Subscribe(TopicLLMUpdated, collector.Handler())
	defer bus.Unsubscribe(updated)

	llm, err := service.CreateLLM("Bedrock", redactFakeKey, "us-east-1", 50, "", "", "", models.BEDROCK, true,
		nil, "anthropic.claude-3", nil, nil, nil, false, redactionTestMetadata(), nil)
	require.NoError(t, err)
	assert.Equal(t, redactFakeKey, llm.APIKey, "the caller keeps the real key")
	_, err = service.UpdateLLM(llm.ID, "Bedrock", models.CredentialRedacted, "us-east-1", 50, "", "", "", models.BEDROCK, true,
		nil, "anthropic.claude-3", nil, nil, nil, "", false, redactionTestMetadata(), nil)
	require.NoError(t, err)
	require.True(t, collector.WaitWithTimeout(2*time.Second))

	for _, ev := range collector.GetEvents() {
		assertNoFakeCredentials(t, string(ev.Payload))
		payload := parseEventPayload(t, ev)
		obj, err := json.Marshal(payload.Object)
		require.NoError(t, err)
		var got models.LLM
		require.NoError(t, json.Unmarshal(obj, &got))
		assert.Equal(t, models.CredentialRedacted, got.APIKey, ev.Topic)
		assert.Equal(t, models.CredentialRedacted, got.Metadata["aws_secret_access_key"], ev.Topic)
		assert.Equal(t, "$SECRET/AWS_SESSION", got.Metadata["aws_session_token"], "references are kept")
		assert.Equal(t, "us-east-1", got.Metadata["aws_region"])
	}

	stored, err := service.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, redactFakeKey, stored.APIKey, "the update's [redacted] key left the key unchanged")
}

// hookPlugin is an object-hook plugin that records what it was sent and
// answers a before-hook with modify (applied to the object it got).
type hookPlugin struct {
	pb.PluginServiceClient
	mu     sync.Mutex
	got    []*pb.ObjectHookRequest
	modify func(map[string]any)
}

func (h *hookPlugin) HandleObjectHook(_ context.Context, req *pb.ObjectHookRequest, _ ...grpc.CallOption) (*pb.ObjectHookResponse, error) {
	h.mu.Lock()
	h.got = append(h.got, req)
	h.mu.Unlock()
	resp := &pb.ObjectHookResponse{AllowOperation: true}
	if h.modify != nil && strings.HasPrefix(req.HookType, "before_") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(req.ObjectJson), &obj); err != nil {
			return nil, err
		}
		h.modify(obj)
		b, _ := json.Marshal(obj)
		resp.Modified, resp.ModifiedObjectJson = true, string(b)
	}
	return resp, nil
}

func (h *hookPlugin) requests() []*pb.ObjectHookRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*pb.ObjectHookRequest(nil), h.got...)
}

func withHookPlugin(t *testing.T, s *Service, plugin *hookPlugin) {
	t.Helper()
	registry := NewHookRegistry()
	require.NoError(t, registry.RegisterHooks(1, "recorder", []*pb.ObjectHookRegistration{{
		ObjectType: string(ObjectTypeLLM),
		HookTypes:  []string{string(HookBeforeCreate), string(HookAfterCreate), string(HookBeforeUpdate), string(HookAfterUpdate)},
	}}))
	manager := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{1: {ID: 1, Name: "recorder", GRPCClient: plugin, IsHealthy: true}}}
	s.HookManager = NewHookManager(registry, manager)
}

// Object hooks, before and after, see an LLM's credentials redacted
// (references kept). A before-hook's result that leaves them redacted keeps
// the real ones; one it sets is used.
func TestObjectHooks_LLMCredentialsRedactedAndRestored(t *testing.T) {
	db := setupDependentsTestDB(t)
	s := NewService(db)
	plugin := &hookPlugin{modify: func(obj map[string]any) {
		obj["short_description"] = "set by the hook"
	}}
	withHookPlugin(t, s, plugin)

	llm, err := s.CreateLLM("Bedrock", redactFakeKey, "us-east-1", 50, "", "", "", models.BEDROCK, true,
		nil, "anthropic.claude-3", nil, nil, nil, false, redactionTestMetadata(), nil)
	require.NoError(t, err)

	reqs := plugin.requests()
	require.NotEmpty(t, reqs)
	for _, req := range reqs {
		assertNoFakeCredentials(t, req.ObjectJson)
		assert.Contains(t, req.ObjectJson, "$SECRET/AWS_SESSION", req.HookType)
	}

	stored, err := s.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "set by the hook", stored.ShortDescription, "the hook's change applies")
	assert.Equal(t, redactFakeKey, stored.APIKey, "the redacted key in its result means unchanged")
	assert.Equal(t, redactFakeSecret, stored.Metadata["aws_secret_access_key"])
	assert.Equal(t, redactFakeAccess, stored.Metadata["aws_access_key_id"])

	// A hook that sets a credential sets it.
	plugin.modify = func(obj map[string]any) {
		obj["metadata"].(map[string]any)["aws_secret_access_key"] = "fake/set+by+the+hook"
	}
	_, err = s.UpdateLLM(llm.ID, "Bedrock", models.CredentialRedacted, "us-east-1", 50, "", "", "", models.BEDROCK, true,
		nil, "anthropic.claude-3", nil, nil, nil, "", false, redactionTestMetadata(), nil)
	require.NoError(t, err)
	stored, err = s.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "fake/set+by+the+hook", stored.Metadata["aws_secret_access_key"])
	assert.Equal(t, redactFakeKey, stored.APIKey)
}
