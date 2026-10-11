// tests/integration/decision_traffic_test.go
//
// Decision models through the assembled microgateway, the edge router that
// serves them in production: real config, service container and server on a
// TCP port, and a mock upstream that speaks both decision wire formats. The
// live vendor suite (make test-vendors-decisions) covers the real APIs; this
// runs in CI on every change.
//
//	client -> /v1/decisions (unified) -> /ai/{slug}/v1/decisions -> /llm/call/{slug}/v1/{decisions|systemone} -> mock
//	client -> /llm/call/{slug}/v1/systemone (native) -> mock
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const decisionTestToken = "e2e-decision-test-token"

// decisionUpstream answers OpenAI /v1/decisions and System One /v1/systemone
// the way the live APIs do (shapes captured by the vendor suite), and records
// what it was sent.
type decisionUpstream struct {
	*httptest.Server
	mu    sync.Mutex
	calls []decisionCall
}

type decisionCall struct {
	Path string
	Auth string
	Body map[string]any
}

func newDecisionUpstream(t *testing.T) *decisionUpstream {
	u := &decisionUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		u.mu.Lock()
		u.calls = append(u.calls, decisionCall{Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: body})
		u.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/systemone"):
			// Jev resolves the alias in its answer and leaves a question out.
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{` +
				`"is_billing":{"type":"noul","noul":0.99},` +
				`"team":{"type":"choice","choice":"billing","confidence":1.0,"probabilities":{"shipping":0.0,"billing":1.0}}},` +
				`"usage":{"input_tokens":400,"output_tokens":72}}`))
		case strings.HasSuffix(r.URL.Path, "/decisions"):
			_, _ = w.Write([]byte(`{"model":"gpt-6-luna","answers":[` +
				`{"type":"predicate","name":"is_billing","probability":1.0},` +
				`{"type":"choice","name":"team","choice":"billing","probabilities":[{"value":"shipping","probability":0.0},{"value":"billing","probability":1.0}],"confidence":1.0},` +
				`{"type":"score","name":"urgency","score":0.54,"probabilities":[{"value":0,"label":"low","probability":0.46},{"value":1,"label":"high","probability":0.54}],"confidence":0.08}],` +
				`"usage":{"input_tokens":261,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens":0,"total_tokens":261}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown path","type":"invalid_request_error"}}`))
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *decisionUpstream) last(t *testing.T) decisionCall {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(t, u.calls, "the upstream was never called")
	return u.calls[len(u.calls)-1]
}

func (u *decisionUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.calls)
}

type decisionEdge struct {
	baseURL string
	db      *gorm.DB
	jev     *mgwdb.LLM
	luna    *mgwdb.LLM
}

// setupDecisionEdge boots the microgateway with two decision routes granted to
// one App: "jev" (System One) and "luna" (OpenAI), plus a priced alias, and an
// ungranted System One route.
func setupDecisionEdge(t *testing.T, upstream *decisionUpstream) *decisionEdge {
	t.Helper()
	dsn := fmt.Sprintf("file:decisions_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, mgwdb.Migrate(db))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: port, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second},
		Database: config.DatabaseConfig{Type: "sqlite"},
		Cache:    config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security: config.SecurityConfig{EncryptionKey: "12345678901234567890123456789012", JWTSecret: "test-secret-key"},
		Analytics: config.AnalyticsConfig{
			Enabled: true, BufferSize: 1, FlushInterval: 100 * time.Millisecond,
			StoreRequestBodies: true, StoreResponseBodies: true, MaxBodySize: 65535,
		},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
	}
	container, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)
	key, err := container.Crypto.Encrypt("upstream-key")
	require.NoError(t, err)

	edge := &decisionEdge{db: db}
	edge.jev = &mgwdb.LLM{Name: "Jev", Slug: "jev", Vendor: "systemone", Endpoint: upstream.URL, APIKeyEncrypted: key, DefaultModel: "jev-latest", IsActive: true}
	edge.luna = &mgwdb.LLM{Name: "Luna", Slug: "luna", Vendor: "openai", Endpoint: upstream.URL + "/v1", APIKeyEncrypted: key, DefaultModel: "gpt-6-luna", IsActive: true}
	hidden := &mgwdb.LLM{Name: "Hidden Jev", Slug: "hidden-jev", Vendor: "systemone", Endpoint: upstream.URL, APIKeyEncrypted: key, DefaultModel: "jev-latest", IsActive: true}
	for _, l := range []*mgwdb.LLM{edge.jev, edge.luna, hidden} {
		require.NoError(t, db.Create(l).Error)
	}
	// Priced under the names the admin configures; Jev answers as jev-1.13.0.
	require.NoError(t, db.Create(&mgwdb.ModelPrice{ModelName: "jev-latest", Vendor: "systemone", CPIT: 0.000000042, Currency: "USD"}).Error)
	require.NoError(t, db.Create(&mgwdb.ModelPrice{ModelName: "gpt-6-luna", Vendor: "openai", CPIT: 0.0000001, Currency: "USD"}).Error)

	app := &mgwdb.App{Name: "Decision Test App", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	for _, l := range []*mgwdb.LLM{edge.jev, edge.luna} {
		require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: l.ID, IsActive: true}).Error)
	}
	require.NoError(t, db.Create(&mgwdb.APIToken{Token: decisionTestToken, Name: "decision-test", AppID: app.ID, IsActive: true}).Error)

	srv, err := server.New(cfg, container, "test", "test", "test")
	require.NoError(t, err)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			t.Logf("server exited: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	edge.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealthy(t, edge.baseURL)
	return edge
}

const decisionBody = `{"model":%q,"input":"I was charged twice for my order (%s).","questions":[` +
	`{"type":"predicate","name":"is_billing","instructions":"Is this about billing?"},` +
	`{"type":"choice","name":"team","instructions":"Which team?","choices":[{"value":"shipping"},{"value":"billing"}]},` +
	`{"type":"score","name":"urgency","instructions":"How urgent?","levels":[{"label":"low"},{"label":"high"}]}]}`

// waitForDecisionEvent polls the edge analytics table for the event whose
// stored request body carries ref.
func waitForDecisionEvent(t *testing.T, db *gorm.DB, llmID uint, ref string) mgwdb.AnalyticsEvent {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var events []mgwdb.AnalyticsEvent
		err := db.Where("llm_id = ? AND request_body LIKE ?", llmID, "%"+ref+"%").Find(&events).Error
		if err == nil && len(events) > 0 && events[0].Name != "" {
			return events[0]
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no analytics event for %s on llm %d", ref, llmID)
	return mgwdb.AnalyticsEvent{}
}

func TestEdgeDecisions_UnifiedIngressToSystemOne(t *testing.T) {
	upstream := newDecisionUpstream(t)
	edge := setupDecisionEdge(t, upstream)

	status, body := gatewayPost(t, edge.baseURL+"/v1/decisions", decisionTestToken, fmt.Sprintf(decisionBody, "jev/jev-latest", "ref-unified"))
	require.Equal(t, http.StatusOK, status, string(body))

	var resp struct {
		Model   string `json:"model"`
		Answers []struct {
			Type, Name string
			Choice     string
		} `json:"answers"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Answers, 3)
	assert.Equal(t, []string{"is_billing", "team", "urgency"}, []string{resp.Answers[0].Name, resp.Answers[1].Name, resp.Answers[2].Name})
	assert.Equal(t, "predicate", resp.Answers[0].Type)
	assert.Equal(t, "billing", resp.Answers[1].Choice)
	assert.Equal(t, "refusal", resp.Answers[2].Type, "the question Jev left out")
	assert.Equal(t, 400, resp.Usage.InputTokens)

	call := upstream.last(t)
	assert.Equal(t, "/v1/systemone", call.Path)
	assert.Equal(t, "Bearer upstream-key", call.Auth, "the route's key, never the App's")
	assert.Equal(t, "jev-latest", call.Body["model"])
	assert.Contains(t, call.Body["state"], "ref-unified")

	ev := waitForDecisionEvent(t, edge.db, edge.jev.ID, "ref-unified")
	assert.Equal(t, "decision", ev.RequestKind)
	assert.Equal(t, 3, ev.DecisionQuestions)
	assert.Equal(t, 1, ev.DecisionRefusals)
	assert.Equal(t, 2, ev.Choices)
	assert.Equal(t, 400, ev.PromptTokens)
	assert.Equal(t, "jev-1.13.0", ev.Name)
	assert.InDelta(t, 400*0.000000042*10000, ev.Cost, 1e-9, "priced by the alias the route asked for")
}

func TestEdgeDecisions_OpenAIPassThroughAndNativeSurface(t *testing.T) {
	upstream := newDecisionUpstream(t)
	edge := setupDecisionEdge(t, upstream)

	t.Run("unified ingress to OpenAI", func(t *testing.T) {
		status, body := gatewayPost(t, edge.baseURL+"/v1/decisions", decisionTestToken, fmt.Sprintf(decisionBody, "luna/gpt-6-luna", "ref-luna"))
		require.Equal(t, http.StatusOK, status, string(body))
		assert.Contains(t, string(body), `"input_tokens_details"`, "OpenAI's body is passed through")
		call := upstream.last(t)
		assert.Equal(t, "/v1/decisions", call.Path)
		assert.Equal(t, "gpt-6-luna", call.Body["model"])

		ev := waitForDecisionEvent(t, edge.db, edge.luna.ID, "ref-luna")
		assert.Equal(t, "decision", ev.RequestKind)
		assert.Equal(t, 0, ev.DecisionRefusals)
		assert.InDelta(t, 261*0.0000001*10000, ev.Cost, 1e-9)
	})

	t.Run("per-route shim", func(t *testing.T) {
		status, body := gatewayPost(t, edge.baseURL+"/ai/jev/v1/decisions", decisionTestToken, fmt.Sprintf(decisionBody, "jev-latest", "ref-shim"))
		require.Equal(t, http.StatusOK, status, string(body))
		assert.Equal(t, "/v1/systemone", upstream.last(t).Path)
	})

	t.Run("native System One passthrough", func(t *testing.T) {
		native := `{"model":"jev-latest","state":"native call (ref-native)","questions":{"is_billing":{"type":"noul","instructions":"?"}}}`
		status, body := gatewayPost(t, edge.baseURL+"/llm/call/jev/v1/systemone", decisionTestToken, native)
		require.Equal(t, http.StatusOK, status, string(body))
		assert.Contains(t, string(body), `"noul"`, "the native surface answers in the vendor's own format")

		ev := waitForDecisionEvent(t, edge.db, edge.jev.ID, "ref-native")
		assert.Equal(t, "decision", ev.RequestKind)
		assert.Equal(t, 1, ev.DecisionQuestions)
	})
}

func TestEdgeDecisions_Refusals(t *testing.T) {
	upstream := newDecisionUpstream(t)
	edge := setupDecisionEdge(t, upstream)

	cases := []struct {
		name   string
		path   string
		token  string
		body   string
		status int
	}{
		{"anonymous", "/v1/decisions", "wrong-token", fmt.Sprintf(decisionBody, "jev/jev-latest", "x"), http.StatusUnauthorized},
		{"ungranted route", "/v1/decisions", decisionTestToken, fmt.Sprintf(decisionBody, "hidden-jev/jev-latest", "x"), http.StatusForbidden},
		{"stream", "/v1/decisions", decisionTestToken, `{"model":"jev/jev-latest","stream":true,"input":"x","questions":[{"type":"predicate","name":"q"}]}`, http.StatusBadRequest},
		{"one-option choice", "/v1/decisions", decisionTestToken, `{"model":"jev/jev-latest","input":"x","questions":[{"type":"choice","name":"q","choices":[{"value":"a"}]}]}`, http.StatusBadRequest},
		{"chat to a decision-only route", "/v1/chat/completions", decisionTestToken, `{"model":"jev/jev-latest","messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest},
		{"native chat path on a decision-only route", "/llm/call/jev/v1/chat/completions", decisionTestToken, `{"model":"jev-latest","messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := gatewayPost(t, edge.baseURL+tc.path, tc.token, tc.body)
			assert.Equal(t, tc.status, status, string(body))
		})
	}
	assert.Zero(t, upstream.count(), "no refused request reaches the vendor")
}

func TestEdgeDecisions_ModelListing(t *testing.T) {
	edge := setupDecisionEdge(t, newDecisionUpstream(t))
	status, body := gatewayGet(t, edge.baseURL+"/v1/models", decisionTestToken)
	require.Equal(t, http.StatusOK, status, string(body))

	var list struct {
		Data []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	caps := map[string][]string{}
	for _, m := range list.Data {
		caps[m.ID] = m.Capabilities
	}
	assert.Equal(t, []string{"decisions"}, caps["jev/jev-latest"])
	assert.Contains(t, caps["luna/gpt-6-luna"], "decisions")
	assert.Contains(t, caps["luna/gpt-6-luna"], "chat")
	_, hidden := caps["hidden-jev/jev-latest"]
	assert.False(t, hidden, "an ungranted route is never listed")
}
