// tests/integration/last_known_good_traffic_test.go
//
// An edge that started from its last-known-good configuration, with no
// control plane, serving real traffic through the assembled microgateway:
// real config, real hub-spoke service container, real server on a TCP port,
// the real authentication path, and a mock OpenAI-compatible upstream.
// See features/ActiveActive.md, Phase 3.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/mockllm"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const lkgTrafficKey = "12345678901234567890123456789012"

// switchableControl is what the gateway asks about a key. While `reachable`
// it answers as a control plane that knows the key (which is how the token
// cache of a running edge gets its entries); otherwise the question goes to
// the real edge client, which has no control plane.
type switchableControl struct {
	real      *edgegrpc.SimpleEdgeClient
	reachable atomic.Bool
	appID     uint32
	asked     atomic.Int32
}

func (s *switchableControl) ValidateTokenOnDemand(token string) (*pb.TokenValidationResponse, error) {
	s.asked.Add(1)
	if s.reachable.Load() {
		return &pb.TokenValidationResponse{Valid: true, AppId: s.appID, AppName: "lkg-app"}, nil
	}
	return s.real.ValidateTokenOnDemand(token)
}

// An edge restarted with no control plane starts from its stored snapshot
// and serves: a request whose key is in the token cache is proxied through
// the restored LLM to the upstream, and a request with an App key the edge
// has to ask a control plane about gets 503 credential_check_unavailable
// from the gateway (not a 401: the key is not known to be wrong).
func TestLastKnownGood_ProxiesCachedKeysAnd503sUnknownOnes(t *testing.T) {
	backend := mockllm.NewMockLLMBackend()
	t.Cleanup(backend.Close)
	path := filepath.Join(t.TempDir(), "edge.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, mgwdb.Migrate(db))
		return db
	}

	// First run, connected: the edge applies (and stores) a snapshot with an
	// LLM pointing at the upstream and an App that may use it.
	first := open()
	require.NoError(t, services.PersistAppliedSnapshots(first, lkgTrafficKey))
	apiKey, err := services.NewCryptoService(lkgTrafficKey).Encrypt("mock-upstream-key")
	require.NoError(t, err)
	created := timestamppb.New(time.Now().Add(-time.Hour))
	snapshot := &pb.ConfigurationSnapshot{
		Version: fmt.Sprint(time.Now().Unix()), Checksum: "lkg-traffic", SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{{
			Id: 1, Name: "Mock OpenAI", Slug: "mock-openai", Vendor: "openai", Endpoint: backend.URL,
			ApiKeyEncrypted: apiKey, DefaultModel: "gpt-4", IsActive: true, CreatedAt: created, UpdatedAt: created,
		}},
		Apps: []*pb.AppConfig{{Id: 7, Name: "lkg-app", IsActive: true, LlmIds: []uint32{1}, CreatedAt: created, UpdatedAt: created}},
	}
	require.NoError(t, services.NewEdgeSyncService(first, "").SyncConfiguration(snapshot))
	require.NoError(t, services.RecordControlContact(first, time.Now()))
	require.NoError(t, mgwdb.Close(first))

	// The edge restarts. Its control endpoint answers nothing.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	controlAddr := closed.Addr().String()
	require.NoError(t, closed.Close())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := &config.Config{
		Server:        config.ServerConfig{Host: "127.0.0.1", Port: port, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second},
		Database:      config.DatabaseConfig{Type: "sqlite"},
		Cache:         config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security:      config.SecurityConfig{EncryptionKey: lkgTrafficKey, JWTSecret: "test-secret-key"},
		Analytics:     config.AnalyticsConfig{Enabled: true, BufferSize: 1, FlushInterval: 100 * time.Millisecond},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
		HubSpoke: config.HubSpokeConfig{
			Mode:                   "edge",
			EdgeID:                 "lkg-traffic-edge",
			ControlEndpoint:        controlAddr,
			ClientToken:            "edge-token",
			AllowInsecure:          true,
			ReconnectInterval:      time.Hour, // no reconnect during the test
			HeartbeatInterval:      time.Hour,
			StartFromLastKnownGood: true,
			StartupControlTimeout:  0,
			LastKnownGoodMaxAge:    72 * time.Hour,
			TokenCacheEnabled:      true,
			TokenCacheTTL:          5 * time.Minute,
			TokenCacheMaxSize:      100,
			TokenCacheCleanupInt:   time.Minute,
		},
	}
	db := open()
	t.Cleanup(func() { _ = mgwdb.Close(db) })
	// Nothing of the first run is left but the stored snapshot: the tables
	// the gateway serves from are refilled by the restore.
	require.NoError(t, db.Exec("DELETE FROM app_llms").Error)
	require.NoError(t, db.Exec("DELETE FROM llms").Error)

	// Start-up, as cmd/microgateway does it.
	client := edgegrpc.NewSimpleEdgeClient(cfg, "test", "test", "test")
	t.Cleanup(func() { _ = client.Stop() })
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.NotNil(t, lkg, "no control plane: the edge starts from its stored snapshot")

	container, err := services.NewHubSpokeServiceContainer(db, cfg)
	require.NoError(t, err)
	require.NoError(t, services.NewEdgeSyncService(db, "").SyncConfiguration(client.GetCurrentConfiguration()))
	hybrid, ok := container.GatewayService.(*services.HybridGatewayService)
	require.True(t, ok, "an edge validates keys through the hybrid gateway service")
	control := &switchableControl{real: client, appID: 7}
	hybrid.SetEdgeClient(control)
	container.ServiceContainer.ConfigSource = client.ConfigSource

	srv, err := server.New(cfg, container.ServiceContainer, "test", "test", "test")
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

	status, body := gatewayGet(t, baseURL+"/ready", "")
	require.Equal(t, http.StatusOK, status, "%s", body)
	var ready map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &ready))
	assert.Equal(t, "last_known_good", ready["config_source"])

	const chat = `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
	const cachedKey = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
	const unknownKey = "0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d"

	// The token cache is seeded the way it is filled in production: by one
	// validation answered while a control plane could be asked.
	control.reachable.Store(true)
	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", cachedKey, chat)
	require.Equal(t, http.StatusOK, status, "seeding the token cache: %s", body)
	control.reachable.Store(false)
	asked := control.asked.Load()
	upstreamBefore := len(backend.GetRequests())

	// No control plane from here on. A request with the cached key is
	// authenticated from the cache and proxied through the restored LLM.
	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", cachedKey, chat)
	require.Equal(t, http.StatusOK, status, "a cached key is served without a control plane: %s", body)
	var completion map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &completion))
	assert.NotEmpty(t, completion["choices"], "the upstream's answer came back through the gateway")
	assert.Equal(t, upstreamBefore+1, len(backend.GetRequests()), "the request reached the upstream")
	assert.Equal(t, asked, control.asked.Load(), "without asking a control plane")

	// An App key the edge has not seen has to be checked with a control
	// plane, and there is none: 503 with Retry-After, which SDKs retry.
	req, err := http.NewRequest(http.MethodPost, baseURL+"/ai/mock-openai/v1/chat/completions", strings.NewReader(chat))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+unknownKey)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	var refusal struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&refusal))
	assert.Equal(t, "credential_check_unavailable", refusal.Error.Code)
	assert.Equal(t, upstreamBefore+1, len(backend.GetRequests()), "nothing went upstream for it")
	assert.Equal(t, asked+1, control.asked.Load(), "the edge did try to ask")
}
