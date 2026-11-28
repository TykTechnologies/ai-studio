// tests/integration/vertex_synced_llm_test.go
//
// A Vertex LLM synced from the control plane, served by the assembled
// microgateway on the native (/llm/rest, /llm/stream) and the
// OpenAI-compatible (/ai, /v1) routes. The LLM's key is a Google credential
// JSON that travels encrypted in the configuration snapshot. The edge
// decrypts it and mints the upstream's access token from it at a fake Google
// token endpoint, which checks that the token request is signed with the
// synced key. A fake Vertex upstream answers.
package integration

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	vertexEdgeKey    = "12345678901234567890123456789012"
	vertexEdgeToken  = "vertex-edge-app-token"
	vertexEdgeSlug   = "vx"
	vertexEdgeModel  = "gemini-2.5-flash"
	vertexSAEmail    = "edge-test@my-proj.iam.gserviceaccount.com"
	vertexSubject    = "oidc-subject-token-from-file"
	vertexNativePath = "/v1/projects/p/locations/us-central1/publishers/google/models/" + vertexEdgeModel
)

// fakeGoogleToken is a Google OAuth2 token endpoint (JWT bearer grant, for
// service account keys) and STS endpoint (token exchange, for Workload
// Identity Federation). It mints only for a JWT signed with pub.
type fakeGoogleToken struct {
	srv *httptest.Server
	pub *rsa.PublicKey

	mu       sync.Mutex
	jwtMints int
	stsMints int
	refusals []string
}

func newFakeGoogleToken(t *testing.T) *fakeGoogleToken {
	t.Helper()
	f := &fakeGoogleToken{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGoogleToken) tokenURL() string { return f.srv.URL + "/token" }

// trust sets the public key a JWT assertion must verify against.
func (f *fakeGoogleToken) trust(pub *rsa.PublicKey) {
	f.mu.Lock()
	f.pub = pub
	f.mu.Unlock()
}
func (f *fakeGoogleToken) stsURL() string { return f.srv.URL + "/sts" }

func (f *fakeGoogleToken) refuse(w http.ResponseWriter, why string) {
	f.mu.Lock()
	f.refusals = append(f.refusals, why)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
}

func (f *fakeGoogleToken) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.refuse(w, "unreadable form")
		return
	}
	var access string
	switch r.URL.Path {
	case "/token":
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			f.refuse(w, "grant_type "+r.Form.Get("grant_type"))
			return
		}
		if why := f.checkAssertion(r.Form.Get("assertion")); why != "" {
			f.refuse(w, why)
			return
		}
		f.mu.Lock()
		f.jwtMints++
		access = fmt.Sprintf("ya29.minted-sa-%d", f.jwtMints)
		f.mu.Unlock()
	case "/sts":
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
			f.refuse(w, "grant_type "+r.Form.Get("grant_type"))
			return
		}
		if r.Form.Get("subject_token") != vertexSubject {
			f.refuse(w, "subject token is not the file's")
			return
		}
		f.mu.Lock()
		f.stsMints++
		access = fmt.Sprintf("ya29.minted-wif-%d", f.stsMints)
		f.mu.Unlock()
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": 3600,
		"issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
	})
}

// checkAssertion verifies a JWT bearer assertion: RS256, signed with the
// test key, issued by the service account for the cloud-platform scope and
// addressed to this token endpoint. It returns why it is refused, or "".
func (f *fakeGoogleToken) checkAssertion(assertion string) string {
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return "assertion is not a JWT"
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if raw, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil || json.Unmarshal(raw, &header) != nil {
		return "unreadable JWT header"
	}
	if header.Alg != "RS256" {
		return "alg " + header.Alg
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "unreadable signature"
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	f.mu.Lock()
	pub := f.pub
	f.mu.Unlock()
	if pub == nil {
		return "no trusted key"
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return "signature does not verify against the synced key"
	}
	var claims struct {
		Iss   string `json:"iss"`
		Aud   string `json:"aud"`
		Scope string `json:"scope"`
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(raw, &claims) != nil {
		return "unreadable claims"
	}
	switch {
	case claims.Iss != vertexSAEmail:
		return "iss " + claims.Iss
	case claims.Aud != f.tokenURL():
		return "aud " + claims.Aud
	case !strings.Contains(claims.Scope, vertexVendor.CloudPlatformScope):
		return "scope " + claims.Scope
	}
	return ""
}

func (f *fakeGoogleToken) counts() (jwt, sts int, refusals []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jwtMints, f.stsMints, append([]string(nil), f.refusals...)
}

// vertexEdgeCall is one request the fake Vertex upstream received.
type vertexEdgeCall struct {
	path, query string
	header      http.Header
}

// fakeVertexUpstream answers generateContent with a JSON object and
// streamGenerateContent with SSE (alt=sse) or a JSON array (otherwise), the
// way Vertex does.
type fakeVertexUpstream struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []vertexEdgeCall
}

func newFakeVertexUpstream(t *testing.T) *fakeVertexUpstream {
	t.Helper()
	f := &fakeVertexUpstream{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeVertexUpstream) serve(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, vertexEdgeCall{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone()})
	f.mu.Unlock()

	finish := `"STOP"`
	if strings.Contains(r.URL.Query().Get("$alt"), "enum-encoding=int") {
		finish = "1"
	}
	chunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello from the edge"}]},"finishReason":` + finish + `}],` +
		`"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":2,"totalTokenCount":8},"modelVersion":"` + vertexEdgeModel + `"}`
	switch {
	case strings.HasSuffix(r.URL.Path, ":generateContent"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chunk)
	case strings.HasSuffix(r.URL.Path, ":streamGenerateContent") && r.URL.Query().Get("alt") == "sse":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+chunk+"\r\n\r\n")
	case strings.HasSuffix(r.URL.Path, ":streamGenerateContent"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "["+chunk+"]")
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeVertexUpstream) received() []vertexEdgeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]vertexEdgeCall(nil), f.calls...)
}

// vertexEdge is a running microgateway whose only LLM is a Vertex LLM synced
// from a control-plane snapshot.
type vertexEdge struct {
	baseURL string
	db      *gorm.DB
}

// startVertexEdge boots the microgateway and applies a snapshot carrying a
// Vertex LLM whose key is credentialJSON, encrypted as the hub sends it.
func startVertexEdge(t *testing.T, upstreamURL, credentialJSON string, llmID uint32) *vertexEdge {
	t.Helper()
	// A file database in WAL mode with a busy timeout, as an edge runs it: the
	// server writes analytics concurrently, and a shared-cache in-memory
	// database answers concurrent writers with "table is locked" instead of
	// waiting.
	dsn := filepath.Join(t.TempDir(), "edge.db") + "?_busy_timeout=5000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgwdb.Close(db) }) // registered first, so it runs after the server stops
	require.NoError(t, mgwdb.Migrate(db))

	// The control plane sends the key encrypted with the shared key; the
	// edge stores it as it arrives.
	keyEnc, err := services.NewCryptoService(vertexEdgeKey).Encrypt(credentialJSON)
	require.NoError(t, err)
	now := timestamppb.New(time.Now().Add(-time.Minute))
	snapshot := &pb.ConfigurationSnapshot{
		Version: fmt.Sprint(time.Now().UnixNano()), Checksum: "vertex-edge", SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{{
			Id: llmID, Name: "Gemini on Vertex", Slug: vertexEdgeSlug, Vendor: "vertex",
			// A URL naming the project and location points the LLM at the fake.
			Endpoint:        upstreamURL + "/v1/projects/p/locations/us-central1",
			ApiKeyEncrypted: keyEnc, DefaultModel: vertexEdgeModel, IsActive: true,
			AllowInternalHost: true, CreatedAt: now, UpdatedAt: now,
		}},
		Apps: []*pb.AppConfig{{Id: 7, Name: "vertex-edge-app", IsActive: true, LlmIds: []uint32{llmID}, CreatedAt: now, UpdatedAt: now}},
	}
	require.NoError(t, services.NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	var stored mgwdb.LLM
	require.NoError(t, db.Where("slug = ?", vertexEdgeSlug).First(&stored).Error)
	require.NotContains(t, stored.APIKeyEncrypted, "private_key", "the key is stored encrypted")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	cfg := &config.Config{
		Server:        config.ServerConfig{Host: "127.0.0.1", Port: port, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second},
		Database:      config.DatabaseConfig{Type: "sqlite"},
		Cache:         config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security:      config.SecurityConfig{EncryptionKey: vertexEdgeKey, JWTSecret: "test-secret-key"},
		Analytics:     config.AnalyticsConfig{Enabled: true, BufferSize: 1, FlushInterval: 100 * time.Millisecond},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
	}
	container, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)
	// The App's token, as a standalone edge stores one (the container sets up
	// token hashing).
	require.NoError(t, db.Create(&mgwdb.APIToken{Token: vertexEdgeToken, Name: "vertex-edge", AppID: 7, IsActive: true}).Error)
	srv, err := server.New(cfg, container, "test", "test", "test")
	require.NoError(t, err)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server exited: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealthy(t, baseURL)
	return &vertexEdge{baseURL: baseURL, db: db}
}

// testServiceAccount returns a service account key JSON whose token_uri is
// the fake token endpoint, and the key's public half.
func testServiceAccount(t *testing.T, tokenURL string) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	sa, err := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "my-proj", "private_key_id": "test-kid",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": vertexSAEmail, "client_id": "1", "token_uri": tokenURL,
	})
	require.NoError(t, err)
	return string(sa), &key.PublicKey
}

// assertOnlyMintedToken checks each upstream call carried exactly the minted
// Bearer and no other credential: not the App's token, not a Google API key.
func assertOnlyMintedToken(t *testing.T, calls []vertexEdgeCall, want string) {
	t.Helper()
	for _, c := range calls {
		assert.Equal(t, []string{"Bearer " + want}, c.header.Values("Authorization"), c.path)
		assert.Empty(t, c.header.Values("X-Goog-Api-Key"), c.path)
		assert.NotContains(t, c.query, "key=", c.path)
		for name, vals := range c.header {
			for _, v := range vals {
				assert.NotContains(t, v, vertexEdgeToken, "header %s carries the App token (%s)", name, c.path)
			}
		}
	}
}

// vertexChatBodies are one buffered and one streamed /ai chat completion.
var vertexChatBodies = []string{
	`{"model":"` + vertexEdgeModel + `","messages":[{"role":"user","content":"hi"}]}`,
	`{"model":"` + vertexEdgeModel + `","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
}

// assertCredentialNotUsable checks a refusal for a credential the edge cannot
// use: 502 with an OpenAI-shaped error that says so, and no value from the
// credential JSON or any token in it.
func assertCredentialNotUsable(t *testing.T, status int, body []byte, credentialJSON string) {
	t.Helper()
	assert.Equal(t, http.StatusBadGateway, status, "%s", body)
	var refusal struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &refusal), "%s", body)
	assert.Contains(t, refusal.Error.Message, "LLM credential not usable", "%s", body)
	assert.NotContains(t, string(body), "ya29.", "no access token in the refusal")
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(credentialJSON), &fields))
	for name, v := range fields {
		if str, ok := v.(string); ok && len(str) > 8 && name != "type" {
			assert.NotContains(t, string(body), str, "the refusal quotes the credential's %s", name)
		}
	}
	assert.NotContains(t, string(body), "PRIVATE KEY")
}

// A Vertex LLM synced from the control plane with a service account key is
// served on all four routes. The edge mints one access token from the synced
// key (the token endpoint verifies the JWT's signature against it) and sends
// only that token upstream.
func TestEdgeVertex_SyncedServiceAccountKeyServesAllRoutes(t *testing.T) {
	t.Cleanup(vertexVendor.AllowLoopbackCredentialEndpointsForTests())
	tokenFake := newFakeGoogleToken(t)
	saJSON, pub := testServiceAccount(t, tokenFake.tokenURL())
	tokenFake.trust(pub)
	upstream := newFakeVertexUpstream(t)
	edge := startVertexEdge(t, upstream.srv.URL, saJSON, 4101)

	native := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	chat := `{"model":"` + vertexEdgeModel + `","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`
	chatStream := `{"model":"` + vertexEdgeModel + `","stream":true,"messages":[{"role":"user","content":"hi"}]}`

	// Native, buffered and streamed.
	status, body := gatewayPost(t, edge.baseURL+"/llm/rest/"+vertexEdgeSlug+vertexNativePath+":generateContent", vertexEdgeToken, native)
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Contains(t, string(body), "hello from the edge")
	status, body = gatewayPost(t, edge.baseURL+"/llm/stream/"+vertexEdgeSlug+vertexNativePath+":streamGenerateContent?alt=sse", vertexEdgeToken, native)
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Contains(t, string(body), "data: ")

	// OpenAI-compatible, buffered and streamed, and the unified /v1 route.
	status, body = gatewayPost(t, edge.baseURL+"/ai/"+vertexEdgeSlug+"/v1/chat/completions", vertexEdgeToken, chat)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(body, &completion), "%s", body)
	require.Len(t, completion.Choices, 1, "%s", body)
	assert.Equal(t, "hello from the edge", completion.Choices[0].Message.Content)
	assert.Equal(t, 6, completion.Usage.PromptTokens)
	assert.Equal(t, 2, completion.Usage.CompletionTokens)

	status, body = gatewayPost(t, edge.baseURL+"/ai/"+vertexEdgeSlug+"/v1/chat/completions", vertexEdgeToken, chatStream)
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Contains(t, string(body), "hello from the edge")
	assert.Contains(t, string(body), "data: [DONE]")

	status, body = gatewayPost(t, edge.baseURL+"/v1/chat/completions", vertexEdgeToken,
		`{"model":"`+vertexEdgeSlug+`/`+vertexEdgeModel+`","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, status, "%s", body)
	assert.Contains(t, string(body), "hello from the edge")

	calls := upstream.received()
	require.Len(t, calls, 5)
	assert.Equal(t, vertexNativePath+":generateContent", calls[0].path)
	assert.Equal(t, vertexNativePath+":streamGenerateContent", calls[1].path)
	assert.Equal(t, "alt=sse", calls[1].query)
	for _, c := range calls[2:] {
		assert.True(t, strings.HasPrefix(c.path, vertexNativePath+":"), "the unified routes reach the Vertex path: %s", c.path)
	}
	assertOnlyMintedToken(t, calls, "ya29.minted-sa-1")

	jwtMints, _, refusals := tokenFake.counts()
	assert.Empty(t, refusals, "every token request was signed with the synced key")
	assert.Equal(t, 1, jwtMints, "one token, reused for every request")

	// Usage is recorded for each request, streamed ones included.
	require.Eventually(t, func() bool {
		var n int64
		edge.db.Model(&mgwdb.AnalyticsEvent{}).Where("prompt_tokens = ? AND response_tokens = ?", 6, 2).Count(&n)
		return n == 5
	}, 5*time.Second, 50*time.Millisecond, "token counts recorded for all five requests")
}

// A key that is not the one the token endpoint trusts is refused there, and
// the edge sends nothing upstream: the request is never sent without a
// token.
func TestEdgeVertex_UntrustedKeyIsNotSentUpstream(t *testing.T) {
	t.Cleanup(vertexVendor.AllowLoopbackCredentialEndpointsForTests())
	tokenFake := newFakeGoogleToken(t)
	_, trusted := testServiceAccount(t, tokenFake.tokenURL())
	tokenFake.trust(trusted)
	otherJSON, _ := testServiceAccount(t, tokenFake.tokenURL())
	upstream := newFakeVertexUpstream(t)
	edge := startVertexEdge(t, upstream.srv.URL, otherJSON, 4102)

	for _, chat := range vertexChatBodies {
		status, body := gatewayPost(t, edge.baseURL+"/ai/"+vertexEdgeSlug+"/v1/chat/completions", vertexEdgeToken, chat)
		assertCredentialNotUsable(t, status, body, otherJSON)
		assert.NotContains(t, string(body), "invalid_grant", "the token endpoint's answer is not passed to the caller")
	}
	status, body := gatewayPost(t, edge.baseURL+"/llm/rest/"+vertexEdgeSlug+vertexNativePath+":generateContent", vertexEdgeToken,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	assert.Equal(t, http.StatusBadGateway, status, "%s", body)
	assert.NotContains(t, string(body), "invalid_grant")

	assert.Empty(t, upstream.received(), "nothing reaches Vertex without a minted token")
	_, _, refusals := tokenFake.counts()
	assert.NotEmpty(t, refusals)
}

// A Workload Identity Federation credential whose subject token is a file on
// the edge is refused unless the operator set AI_STUDIO_VERTEX_ADC=true; with
// it, the edge exchanges the file's token at STS and sends the result.
func TestEdgeVertex_WIFCredentialFollowsADCSetting(t *testing.T) {
	t.Cleanup(vertexVendor.AllowLoopbackCredentialEndpointsForTests())
	tokenFake := newFakeGoogleToken(t)
	subjectFile := filepath.Join(t.TempDir(), "subject-token")
	require.NoError(t, os.WriteFile(subjectFile, []byte(vertexSubject), 0o600))
	wif, err := json.Marshal(map[string]any{
		"type":               "external_account",
		"audience":           "//iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/pool/providers/prov",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          tokenFake.stsURL(),
		"credential_source":  map[string]any{"file": subjectFile},
	})
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		setting string
		llmID   uint32
	}{
		{"setting unset", "", 4103},
		{"setting false", "false", 4104},
		{"setting true", "true", 4105},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(vertexVendor.ADCSetting, tc.setting)
			upstream := newFakeVertexUpstream(t)
			edge := startVertexEdge(t, upstream.srv.URL, string(wif), tc.llmID)
			_, stsBefore, _ := tokenFake.counts()

			if tc.setting != "true" {
				for _, chat := range vertexChatBodies {
					status, body := gatewayPost(t, edge.baseURL+"/ai/"+vertexEdgeSlug+"/v1/chat/completions", vertexEdgeToken, chat)
					assertCredentialNotUsable(t, status, body, string(wif))
					assert.Contains(t, string(body), vertexVendor.ADCSetting, "the refusal names the setting that allows it")
					assert.NotContains(t, string(body), subjectFile, "nor the host file it would read")
				}
				_, stsAfter, _ := tokenFake.counts()
				assert.Empty(t, upstream.received(), "a host-sourced credential is not used without the setting")
				assert.Equal(t, stsBefore, stsAfter, "and the host's token is not exchanged")
				return
			}
			status, body := gatewayPost(t, edge.baseURL+"/ai/"+vertexEdgeSlug+"/v1/chat/completions", vertexEdgeToken, vertexChatBodies[0])
			_, stsAfter, _ := tokenFake.counts()
			require.Equal(t, http.StatusOK, status, "%s", body)
			assert.Contains(t, string(body), "hello from the edge")
			assert.Equal(t, stsBefore+1, stsAfter)
			calls := upstream.received()
			require.Len(t, calls, 1)
			assertOnlyMintedToken(t, calls, fmt.Sprintf("ya29.minted-wif-%d", stsAfter))
		})
	}
}
