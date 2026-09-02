// tests/integration/streaming_metrics_test.go
//
// Verifies the OpenTelemetry GenAI streaming metrics are actually wired into the
// request path, not merely registered. Unit tests cover the instruments and the
// timing struct in isolation; what needs an assembled gateway is the plumbing —
// that markFirstChunk is reached inside the real SSE read loop, and that the
// timing survives onto the context the analytics path reads.
//
// Uses a raw SSE upstream rather than the mock LLM backend, which only speaks
// buffered JSON.
package integration

import (
	"context"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const streamingTestToken = "e2e-streaming-metrics-token"

// upstreamHeaders captures the headers the model server actually received, so
// trace propagation can be asserted from the far side rather than inferred.
type upstreamHeaders struct {
	mu   sync.Mutex
	last http.Header
}

func (u *upstreamHeaders) record(h http.Header) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.last = h.Clone()
}

func (u *upstreamHeaders) get(key string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.last == nil {
		return ""
	}
	return u.last.Get(key)
}

// newSSEUpstream serves an OpenAI-style streamed completion, with a deliberate
// pause before the first chunk so time-to-first-token is measurably non-zero and
// distinguishable from total duration.
func newSSEUpstream(t *testing.T, firstTokenDelay time.Duration, seen *upstreamHeaders) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.record(r.Header)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "test server must support flushing")

		time.Sleep(firstTokenDelay)

		for i, word := range []string{"hello", " ", "there"} {
			fmt.Fprintf(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\","+
				"\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", word)
			flusher.Flush()
			// Space the chunks so each arrives as its own Read on the gateway
			// side; without this they coalesce and a per-chunk regression in the
			// time-to-first-token accounting would be invisible.
			_ = i
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprint(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\","+
			"\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],"+
			"\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":3,\"total_tokens\":14}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupStreamingMetricsTest boots the gateway with metrics enabled and one LLM
// pointing at the SSE upstream.
func setupStreamingMetricsTest(t *testing.T, upstreamURL string) string {
	t.Helper()

	dsn := fmt.Sprintf("file:streammetrics_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, mgwdb.Migrate(db))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         port,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: config.DatabaseConfig{Type: "sqlite"},
		Cache:    config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
			JWTSecret:     "test-secret-key",
		},
		Analytics: config.AnalyticsConfig{
			Enabled:       true,
			BufferSize:    1,
			FlushInterval: 100 * time.Millisecond,
		},
		Observability: config.ObservabilityConfig{
			LogLevel:                    "error",
			LogFormat:                   "json",
			EnableMetrics:               true,
			MetricsPath:                 "/metrics",
			MetricsAllowUnauthenticated: true,
		},
	}

	serviceContainer, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)

	apiKeyEnc, err := serviceContainer.Crypto.Encrypt("sse-upstream-key")
	require.NoError(t, err)

	llm := &mgwdb.LLM{
		Name:            "SSE Model",
		Slug:            "sse-model",
		Vendor:          "openai",
		Endpoint:        upstreamURL,
		APIKeyEncrypted: apiKeyEnc,
		DefaultModel:    "gpt-4",
		IsActive:        true,
	}
	require.NoError(t, db.Create(llm).Error)

	app := &mgwdb.App{Name: "Streaming Metrics App", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error)
	require.NoError(t, db.Create(&mgwdb.APIToken{
		Token:    streamingTestToken,
		Name:     "streaming-metrics-test",
		AppID:    app.ID,
		IsActive: true,
	}).Error)

	srv, err := server.New(cfg, serviceContainer, "test", "test", "test")
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

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealthy(t, baseURL)
	return baseURL
}

func TestStreamingGenAIMetrics(t *testing.T) {
	seen := &upstreamHeaders{}
	upstream := newSSEUpstream(t, 60*time.Millisecond, seen)
	baseURL := setupStreamingMetricsTest(t, upstream.URL)

	// Ordering matters here. metrics.Init installs a fresh global registry per
	// gateway, so the non-streaming assertion has to run against the same
	// gateway and before any streaming traffic, or it proves nothing.
	t.Run("a non-streaming request emits no streaming metrics", func(t *testing.T) {
		nonStreaming := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
		gatewayPost(t, baseURL+"/llm/rest/sse-model/v1/chat/completions", streamingTestToken, nonStreaming)

		_, b := gatewayGet(t, baseURL+"/metrics", "")
		body := string(b)

		require.Contains(t, body, "gen_ai_server_request_duration_seconds",
			"the non-streaming request should still record a duration")
		assert.NotContains(t, body, "gen_ai_server_time_to_first_token_seconds",
			"time-to-first-token must be streaming-only")
		assert.NotContains(t, body, "gen_ai_server_time_per_output_token_seconds",
			"time-per-output-token must be streaming-only")
	})

	// The gateway rejects streaming without usage reporting, since it could not
	// otherwise meter the request.
	body := `{"model":"gpt-4","stream":true,"stream_options":{"include_usage":true},` +
		`"messages":[{"role":"user","content":"hello"}]}`
	status, respBody := gatewayPost(t, baseURL+"/llm/stream/sse-model/v1/chat/completions", streamingTestToken, body)
	require.Equal(t, http.StatusOK, status, "body: %s", respBody)
	require.Contains(t, string(respBody), "hello", "the stream did not reach the client: %s", respBody)

	// Analytics — and with it time-per-output-token — is recorded asynchronously.
	var metricsBody string
	require.Eventually(t, func() bool {
		_, b := gatewayGet(t, baseURL+"/metrics", "")
		metricsBody = string(b)
		return strings.Contains(metricsBody, "gen_ai_server_time_to_first_token_seconds")
	}, 5*time.Second, 100*time.Millisecond, "time-to-first-token was never recorded")

	t.Run("an inbound trace is propagated to the model server", func(t *testing.T) {
		// The claim being tested: a caller's trace survives the hop through the
		// gateway even with span export disabled, because the W3C propagator is
		// installed unconditionally. Asserting at the far side is what catches a
		// director that drops the header.
		const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
		req, err := http.NewRequest(http.MethodPost,
			baseURL+"/llm/stream/sse-model/v1/chat/completions", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+streamingTestToken)
		req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)

		got := seen.get("traceparent")
		require.NotEmpty(t, got, "the upstream never received a traceparent header")
		assert.Contains(t, got, traceID,
			"the upstream received a different trace; the caller's trace was not continued")
	})

	t.Run("streaming instruments are populated", func(t *testing.T) {
		assert.Contains(t, metricsBody, "gen_ai_server_time_to_first_token_seconds_count",
			"markFirstChunk was never reached in the SSE read loop")
		assert.Contains(t, metricsBody, "gen_ai_server_request_duration_seconds")
		assert.Contains(t, metricsBody, `gen_ai_provider_name="openai"`)
	})

	t.Run("time to first token is recorded exactly once per stream", func(t *testing.T) {
		// The deterministic guard against the obvious regression — recording on
		// every chunk instead of the first. The upstream emits four chunks, so a
		// per-chunk bug shows up as a count of four. A sum comparison would not
		// catch it reliably, since chunks can coalesce into one read.
		count := histogramSum(t, metricsBody, "gen_ai_server_time_to_first_token_seconds_count")
		assert.Equal(t, 1.0, count,
			"expected one time-to-first-token observation for one streaming request")
	})

	t.Run("time to first token is below total duration", func(t *testing.T) {
		// The upstream stalls 60ms before the first chunk and then streams, so
		// the two histograms must not be recording the same instant.
		ttft := histogramSum(t, metricsBody, "gen_ai_server_time_to_first_token_seconds_sum")
		total := histogramSum(t, metricsBody, "gen_ai_server_request_duration_seconds_sum")
		assert.Greater(t, ttft, 0.0, "time to first token should be measurably non-zero")
		assert.LessOrEqual(t, ttft, total, "time to first token cannot exceed total request duration")
	})

	t.Run("time per output token is recorded once tokens are known", func(t *testing.T) {
		// TPOT needs the completion token count, which only arrives via the
		// analytics path — the part of the plumbing most likely to break.
		require.Eventually(t, func() bool {
			_, b := gatewayGet(t, baseURL+"/metrics", "")
			return strings.Contains(string(b), "gen_ai_server_time_per_output_token_seconds_count")
		}, 5*time.Second, 100*time.Millisecond,
			"time-per-output-token never recorded; the stream timing did not reach the analytics path")
	})
}

// histogramSum reads the value of a single Prometheus sample line by name.
func histogramSum(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name) {
			continue
		}
		idx := strings.LastIndex(line, " ")
		if idx < 0 {
			continue
		}
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSpace(line[idx:]), "%g", &v); err == nil {
			return v
		}
	}
	t.Fatalf("no sample found for %s in:\n%s", name, truncateForLog(body))
	return 0
}

func truncateForLog(s string) string {
	const max = 4000
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}
