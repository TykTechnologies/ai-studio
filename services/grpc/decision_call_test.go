package grpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDecisionCallBody(t *testing.T) {
	body, err := decisionCallBody(`{"input":"x","questions":[{"type":"predicate","name":"q"}],"metadata":{"a":1}}`, "jev-latest")
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &got))
	assert.JSONEq(t, `"jev-latest"`, string(got["model"]), "the LLM's default fills an empty model")
	assert.JSONEq(t, `{"a":1}`, string(got["metadata"]), "unknown fields are kept")

	body, err = decisionCallBody(`{"model":"jev-1.13.0","input":"x","questions":[]}`, "jev-latest")
	require.NoError(t, err)
	assert.Contains(t, string(body), `"jev-1.13.0"`)

	_, err = decisionCallBody(`{"input":"x"}`, "")
	assert.ErrorContains(t, err, "no default model")
	_, err = decisionCallBody(`[1,2]`, "m")
	assert.Error(t, err)
}

func TestDecisionCallError(t *testing.T) {
	assert.Equal(t, "decision call failed (status 403): Budget limit exceeded",
		decisionCallError(403, []byte(`{"error":{"message":"Budget limit exceeded","type":"permission_error"}}`)))
	assert.Equal(t, "decision call failed (status 502): bad gateway", decisionCallError(502, []byte("bad gateway\n")))
}

// agentFixture is an agent plugin whose App holds one LLM ("Jev") but not
// another ("Other").
type agentFixture struct {
	server *AIStudioManagementServer
	ctx    context.Context
	held   *models.LLM
	other  *models.LLM
}

func seedAgent(t *testing.T, scopes ...string) agentFixture {
	t.Helper()
	server, service, plugin, ctx := setupExtensionRPCTest(t, scopes...)
	db := service.DB
	f := agentFixture{server: server, ctx: ctx}
	f.held = &models.LLM{Name: "Jev", Vendor: models.SYSTEMONE, DefaultModel: "jev-latest", Active: true}
	f.other = &models.LLM{Name: "Other", Vendor: models.SYSTEMONE, Active: true}
	require.NoError(t, db.Create(f.held).Error)
	require.NoError(t, db.Create(f.other).Error)
	cred := &models.Credential{KeyID: "k", Secret: "s3cret", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "agent-app", CredentialID: cred.ID, LLMs: []models.LLM{*f.held}}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&models.AgentConfig{Name: "a", Slug: "a", PluginID: plugin.ID, AppID: app.ID}).Error)
	return f
}

// fakeGateway stands in for the embedded gateway an agent call goes through.
type fakeGateway struct {
	mu     sync.Mutex
	path   string
	auth   string
	body   map[string]any
	status int
	reply  string
}

func useFakeGateway(t *testing.T, status int, reply string) *fakeGateway {
	t.Helper()
	g := &fakeGateway{status: status, reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.path, g.auth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.Unmarshal(raw, &g.body)
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(g.status)
		_, _ = w.Write([]byte(g.reply))
	}))
	prev := agentProxyBaseURL
	agentProxyBaseURL = srv.URL
	t.Cleanup(func() {
		agentProxyBaseURL = prev
		srv.Close()
	})
	return g
}

func TestCallDecision_AccessChecks(t *testing.T) {
	t.Run("needs the llms.proxy scope", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeKVReadWrite)
		_, err := server.CallDecision(ctx, &pb.CallDecisionRequest{LlmId: 1, RequestJson: `{}`})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("needs an agent configuration", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeLLMsProxy)
		_, err := server.CallDecision(ctx, &pb.CallDecisionRequest{LlmId: 1, RequestJson: `{}`})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("only LLMs the agent's App holds", func(t *testing.T) {
		f := seedAgent(t, models.ServiceScopeLLMsProxy)
		_, err := f.server.CallDecision(f.ctx, &pb.CallDecisionRequest{LlmId: uint32(f.other.ID), RequestJson: `{}`})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))

		_, err = f.server.CallDecision(f.ctx, &pb.CallDecisionRequest{LlmId: uint32(f.held.ID), RequestJson: `not json`})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "a bad request is the caller's error, before any call")
	})
}

func TestCallDecision_GoesThroughTheGatewayAsTheAgentsApp(t *testing.T) {
	f := seedAgent(t, models.ServiceScopeLLMsProxy)

	t.Run("success", func(t *testing.T) {
		g := useFakeGateway(t, http.StatusOK, `{"model":"jev-1.13.0","answers":[{"type":"predicate","name":"q","probability":0.9}],"usage":{"input_tokens":5,"output_tokens":0}}`)
		resp, err := f.server.CallDecision(f.ctx, &pb.CallDecisionRequest{
			LlmId: uint32(f.held.ID), RequestJson: `{"input":"x","questions":[{"type":"predicate","name":"q"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.Success, resp.ErrorMessage)
		assert.Contains(t, resp.ResponseJson, `"answers"`)
		assert.Equal(t, "/ai/jev/v1/decisions", g.path)
		assert.Equal(t, "Bearer s3cret", g.auth, "the agent App's credential")
		assert.Equal(t, "jev-latest", g.body["model"], "the LLM's default model")
	})

	t.Run("a gateway refusal reaches the plugin", func(t *testing.T) {
		useFakeGateway(t, http.StatusForbidden, `{"error":{"message":"Budget limit exceeded","type":"permission_error"}}`)
		resp, err := f.server.CallDecision(f.ctx, &pb.CallDecisionRequest{
			LlmId: uint32(f.held.ID), RequestJson: `{"input":"x","questions":[{"type":"predicate","name":"q"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, int32(http.StatusForbidden), resp.StatusCode)
		assert.Contains(t, resp.ErrorMessage, "Budget limit exceeded")
	})

	t.Run("an unreachable gateway is a failed call, not an RPC error", func(t *testing.T) {
		prev := agentProxyBaseURL
		agentProxyBaseURL = "http://127.0.0.1:1"
		defer func() { agentProxyBaseURL = prev }()
		resp, err := f.server.CallDecision(f.ctx, &pb.CallDecisionRequest{
			LlmId: uint32(f.held.ID), RequestJson: `{"input":"x","questions":[{"type":"predicate","name":"q"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.ErrorMessage, "proxy call failed")
	})
}

// fakeCallLLMStream is the server side of CallLLM's response stream.
type fakeCallLLMStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*pb.CallLLMResponse
}

func (s *fakeCallLLMStream) Context() context.Context { return s.ctx }
func (s *fakeCallLLMStream) Send(r *pb.CallLLMResponse) error {
	s.sent = append(s.sent, r)
	return nil
}

// CallLLM now shares its access checks with CallDecision (agentLLMForPlugin).
// These pin that the refactor kept its behaviour.
func TestCallLLM_AccessChecksAndGatewayCall(t *testing.T) {
	t.Run("needs the llms.proxy scope", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeKVReadWrite)
		err := server.CallLLM(&pb.CallLLMRequest{LlmId: 1}, &fakeCallLLMStream{ctx: ctx})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("needs an agent configuration", func(t *testing.T) {
		server, _, _, ctx := setupExtensionRPCTest(t, models.ServiceScopeLLMsProxy)
		err := server.CallLLM(&pb.CallLLMRequest{LlmId: 1}, &fakeCallLLMStream{ctx: ctx})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	f := seedAgent(t, models.ServiceScopeLLMsProxy)

	t.Run("only LLMs the agent's App holds", func(t *testing.T) {
		err := f.server.CallLLM(&pb.CallLLMRequest{LlmId: uint32(f.other.ID)}, &fakeCallLLMStream{ctx: f.ctx})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("a held LLM is called through the gateway", func(t *testing.T) {
		g := useFakeGateway(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
		stream := &fakeCallLLMStream{ctx: f.ctx}
		err := f.server.CallLLM(&pb.CallLLMRequest{
			LlmId: uint32(f.held.ID), Model: "jev-latest",
			Messages: []*pb.LLMMessage{{Role: "user", Content: "hi"}},
		}, stream)
		require.NoError(t, err)
		require.Len(t, stream.sent, 1)
		assert.True(t, stream.sent[0].Success)
		assert.Equal(t, "hello", stream.sent[0].Content)
		assert.Equal(t, "/ai/jev/v1/chat/completions", g.path)
		assert.Equal(t, "Bearer s3cret", g.auth)
	})
}
