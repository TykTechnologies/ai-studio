//go:build vendorlive

// Package vendorconformance runs the live vendor conformance suite against the
// microgateway data plane: the shim, sdk and universal ingress surfaces.
//
// It is gated twice over — the `vendorlive` build tag AND
// VENDOR_TESTS_ENABLED=true — so `go test ./...`, `make test-all` and CI cannot
// pick it up by accident. It calls real vendors and spends real money.
//
//	make test-vendors
//
// Credentials come from test-secrets/vendors.env; see
// features/VendorConformance.md for the design and test-secrets/README.md for
// setup.
package vendorconformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testAPIToken = "vendor-conformance-token"

// harness is a running microgateway with every configured vendor seeded as an
// LLM route, shared by all tests in the package. Booting once matters: each boot
// costs a server start and a route-table load, and the gateway resolves its
// routes at construction time so vendors cannot be added later anyway.
type harness struct {
	cfg      *vc.Config
	baseURL  string
	recorder *vc.Recorder
	client   *http.Client
	// inflight caps concurrent upstream calls. Vendor rate limits are per
	// account, not per test, so unbounded t.Parallel() fan-out turns a working
	// suite into a wall of 429s that look exactly like product failures.
	inflight chan struct{}
}

// maxRateLimitRetries bounds the retry loop. Rate limits are transient and
// worth waiting out; anything still failing after this is a real result.
const maxRateLimitRetries = 3

// maxRateLimitWait caps a single backoff. Vendors sometimes ask for a minute.
const maxRateLimitWait = 70 * time.Second

// CallContext returns the overall budget for one logical call, including any
// rate-limit backoff.
//
// VENDOR_TESTS_TIMEOUT is the budget for a single ATTEMPT, not for the whole
// retrying call. Conflating the two is a trap: a vendor that answers "retry in
// 58s" would consume half a 120s deadline waiting, then get cancelled mid-retry
// and report as a timeout rather than as the rate limit it is.
func (h *harness) CallContext(parent context.Context) (context.Context, context.CancelFunc) {
	budget := h.cfg.Timeout + maxRateLimitRetries*(maxRateLimitWait+h.cfg.Timeout)
	return context.WithTimeout(parent, budget)
}

// attemptContext bounds one HTTP attempt within the overall call budget.
func (h *harness) attemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, h.cfg.Timeout)
}

// acquire blocks until a request slot is free.
func (h *harness) acquire() func() {
	h.inflight <- struct{}{}
	return func() { <-h.inflight }
}

// isRateLimited reports whether an exchange failed because of an upstream rate
// limit, and how long to wait before retrying.
//
// The status alone is not enough: an upstream 429 reaches us as a 500 or 502
// wrapped in an error envelope (the gateway does not currently map upstream
// status onto its own), and a 429 during a stream arrives as a 200 whose first
// SSE frame is an error object. Both are rate limits and neither looks like one.
func isRateLimited(status int, body []byte) (bool, time.Duration) {
	if status == http.StatusOK && !looksLikeErrorEnvelope(body) {
		return false, 0
	}
	text := string(body)
	if !strings.Contains(text, "429") &&
		!strings.Contains(strings.ToLower(text), "rate limit") &&
		!strings.Contains(strings.ToLower(text), "quota") &&
		!strings.Contains(strings.ToLower(text), "too many requests") {
		return false, 0
	}

	wait := 20 * time.Second
	// Vendors often name the exact wait ("Please retry in 58.70330495s.").
	if m := retryAfterPattern.FindStringSubmatch(text); len(m) == 2 {
		if d, err := time.ParseDuration(m[1] + "s"); err == nil && d > 0 && d < 5*time.Minute {
			wait = d + 2*time.Second
		}
	}
	if wait > maxRateLimitWait {
		wait = maxRateLimitWait
	}
	return true, wait
}

var retryAfterPattern = regexp.MustCompile(`retry in ([0-9]+(?:\.[0-9]+)?)s`)

// modelCapabilityGap reports whether an upstream error says this MODEL cannot do
// what the scenario asked — as opposed to our translation being wrong.
//
// The distinction matters and is easy to get wrong in both directions. A model
// that rejects function calling outright ("Function tools ... are not supported
// for <model>") tells us nothing about whether our tool translation is correct,
// so failing on it would be noise that trains people to ignore the suite. But
// the check is deliberately narrow — it keys on the vendor explicitly naming an
// unsupported feature, not on any 400 — because a translation bug that produces
// a malformed request also returns 400, and that one must fail loudly.
func modelCapabilityGap(body []byte) (string, bool) {
	text := string(body)
	markers := []string{
		"are not supported for",
		"is not supported for",
		"does not support",
		"not supported with this model",
		"unsupported_value",
	}
	for _, m := range markers {
		if idx := strings.Index(text, m); idx >= 0 {
			start := idx - 120
			if start < 0 {
				start = 0
			}
			end := idx + 200
			if end > len(text) {
				end = len(text)
			}
			return strings.TrimSpace(text[start:end]), true
		}
	}
	return "", false
}

// skipIfThrottled reports whether the vendor refused to serve us for reasons
// that say nothing about our translation, skipping the test if so.
//
// A rate limit or a capacity refusal is not a conformance signal. Free-tier
// Gemini quotas in particular will paint a whole run red for a reason that has
// nothing to do with the product, and a suite that cries wolf gets ignored.
func skipIfThrottled(t *testing.T, resp *response) bool {
	t.Helper()
	switch resp.Status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		t.Skipf("upstream returned %d (rate limit or capacity), not a conformance signal:\n%s",
			resp.Status, bodyExcerpt(resp.Body))
		return true
	}
	return false
}

// looksLikeErrorEnvelope reports whether a payload is an {"error": ...} object
// rather than a completion. A 200 response whose body is an error envelope is
// how a mid-stream upstream failure reaches the client.
func looksLikeErrorEnvelope(body []byte) bool {
	var probe struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return len(probe.Error) > 0
}

var (
	shared     *harness
	sharedOnce sync.Once
	sharedSkip string
)

// TestMain boots the harness once, runs everything, then flushes artifacts.
func TestMain(m *testing.M) {
	code := m.Run()
	if shared != nil && shared.recorder != nil {
		summary, err := shared.recorder.Flush()
		if err != nil {
			fmt.Fprintf(os.Stderr, "vendorconformance: writing artifacts: %v\n", err)
		} else if summary != "" {
			fmt.Fprintf(os.Stderr, "\n=== VENDOR CONFORMANCE ARTIFACTS ===\n  %s\n", summary)
		}
	}
	os.Exit(code)
}

// setup returns the shared harness, skipping the calling test if the suite is
// not enabled or no vendor has credentials.
func setup(t *testing.T) *harness {
	t.Helper()
	sharedOnce.Do(func() {
		h, skip := buildHarness()
		shared, sharedSkip = h, skip
	})
	if sharedSkip != "" {
		t.Skip(sharedSkip)
	}
	return shared
}

func buildHarness() (*harness, string) {
	cfg, err := vc.Load()
	if err != nil {
		// A malformed config is a failure, not a skip — but TestMain has no
		// *testing.T, so it surfaces as a skip carrying the error. Every test
		// prints it, so it cannot be missed.
		return nil, "vendor conformance config error: " + err.Error()
	}
	if !cfg.Enabled {
		return nil, "VENDOR_TESTS_ENABLED is not true (set it in test-secrets/vendors.env)"
	}
	if len(cfg.Vendors) == 0 {
		return nil, "no vendors have credentials:\n" + cfg.Summary()
	}

	fmt.Fprint(os.Stderr, "\n"+cfg.Summary()+"\n")

	h := &harness{
		cfg:      cfg,
		recorder: vc.NewRecorder(cfg.ArtifactDir),
		// No client timeout: per-request deadlines come from the context, so a
		// slow vendor produces a clear context error rather than an opaque one.
		client:   &http.Client{Timeout: 0},
		inflight: make(chan struct{}, cfg.Parallel),
	}
	if err := h.boot(); err != nil {
		return nil, "booting microgateway: " + err.Error()
	}
	return h, ""
}

// boot starts an in-process microgateway seeded with every configured vendor.
//
// It mirrors microgateway/tests/integration/gateway_traffic_test.go: a real
// listener on a free port is required because the /ai/ shim's internal
// /llm/call/ hop is a genuine HTTP call to 127.0.0.1:<port>.
func (h *harness) boot() error {
	dsn := fmt.Sprintf("file:vendorconf_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("opening sqlite: %w", err)
	}
	if err := mgwdb.Migrate(db); err != nil {
		return fmt.Errorf("migrating: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return err
	}

	timeout := h.cfg.Timeout
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: port,
			// Long-context and reasoning responses routinely outlast the 30s
			// used by the mock-backed integration tests.
			ReadTimeout:  timeout + 30*time.Second,
			WriteTimeout: timeout + 30*time.Second,
		},
		Database: config.DatabaseConfig{Type: "sqlite"},
		Cache:    config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
			JWTSecret:     "vendor-conformance-secret",
		},
		Analytics:     config.AnalyticsConfig{Enabled: false},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
	}

	container, err := services.NewServiceContainer(db, cfg)
	if err != nil {
		return fmt.Errorf("building service container: %w", err)
	}

	app := &mgwdb.App{Name: "Vendor Conformance", IsActive: true}
	if err := db.Create(app).Error; err != nil {
		return err
	}

	// Seed BEFORE server.New: the gateway loads its route table once, during
	// construction. An LLM created afterwards is invisible to routing.
	for _, v := range h.cfg.Vendors {
		llm, err := seedLLM(db, container, v, h.cfg.MaxTokens)
		if err != nil {
			return fmt.Errorf("seeding %s: %w", v.Key, err)
		}
		if err := db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error; err != nil {
			return err
		}
	}

	if err := db.Create(&mgwdb.APIToken{
		Token: testAPIToken, Name: "vendor-conformance", AppID: app.ID, IsActive: true,
	}).Error; err != nil {
		return err
	}

	srv, err := server.New(cfg, container, "test", "test", "test")
	if err != nil {
		return fmt.Errorf("building server: %w", err)
	}
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "vendorconformance: gateway exited: %v\n", err)
		}
	}()

	h.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	return h.waitHealthy()
}

func (h *harness) waitHealthy() error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(h.baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("gateway never became healthy at %s", h.baseURL)
}

// seedLLM writes one vendor's configuration as a data-plane LLM row.
//
// Vendor-specific config lives in different places for different vendors —
// Bedrock's AWS credentials in Metadata, Vertex's "project:location" in
// Endpoint — so this is the one place that has to know those shapes. Getting it
// wrong produces auth failures that look exactly like translation bugs, which is
// why the mapping is spelled out rather than inferred.
func seedLLM(db *gorm.DB, container *services.ServiceContainer, v vc.VendorConfig, maxTokens int) (*mgwdb.LLM, error) {
	defaultModel, _ := v.Model(vc.ModelLatest)

	llm := &mgwdb.LLM{
		Name:           "VT " + v.Key,
		Slug:           v.Slug(),
		Vendor:         string(v.Vendor),
		Endpoint:       v.Endpoint,
		DefaultModel:   defaultModel,
		MaxTokens:      maxTokens,
		TimeoutSeconds: 300,
		RetryCount:     0, // a retry would mask a transient failure we want to see
		IsActive:       true,
		// AllowedModels is deliberately left empty: an empty pattern list allows
		// every model (see proxy.ModelValidator.IsModelAllowed), and the errors
		// scenario asserts the deny path explicitly rather than relying on it here.
	}

	if v.APIKey != "" {
		enc, err := container.Crypto.Encrypt(v.APIKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting API key: %w", err)
		}
		llm.APIKeyEncrypted = enc
	}

	if len(v.Metadata) > 0 {
		meta, err := json.Marshal(v.Metadata)
		if err != nil {
			return nil, err
		}
		llm.Metadata = meta
	}

	if err := db.Create(llm).Error; err != nil {
		return nil, err
	}
	return llm, nil
}

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

// response is one completed HTTP exchange with the gateway.
type response struct {
	Status int
	Body   []byte
	Header http.Header
}

// URLFor returns the gateway URL for a surface. The paths are the real mounted
// routes, not a reimplementation: see proxy.createHandler.
func (h *harness) URLFor(surface vc.Surface, v vc.VendorConfig) string {
	switch surface {
	case vc.SurfaceShim:
		return fmt.Sprintf("%s/ai/%s/v1/chat/completions", h.baseURL, v.Slug())
	case vc.SurfaceUniversal:
		return h.baseURL + "/v1/chat/completions"
	case vc.SurfaceSDK:
		return fmt.Sprintf("%s/llm/call/%s/%s", h.baseURL, v.Slug(), nativePath(v))
	default:
		panic("vendorconformance: no gateway URL for surface " + string(surface))
	}
}

// nativePath is the vendor-native suffix appended to the LLM's own endpoint by
// the /llm/call/ passthrough. Each vendor's SDK speaks a different path, which
// is precisely what the sdk surface exists to exercise.
//
// The version segment is derived from the configured endpoint rather than
// hardcoded, because the endpoint's shape is not ours to dictate: Anthropic's
// must carry /v1 for the langchaingo driver, so hardcoding "v1/messages" here
// would proxy to /v1/v1/messages.
func nativePath(v vc.VendorConfig) string {
	endsWithVersion := func(seg string) bool {
		return strings.HasSuffix(strings.TrimSuffix(v.Endpoint, "/"), "/"+seg)
	}

	switch v.Vendor {
	case "anthropic":
		if endsWithVersion("v1") {
			return "messages"
		}
		return "v1/messages"
	case "google_ai":
		version := v.Extra["api_version"]
		if version == "" {
			version = "v1beta"
		}
		// The model is part of the path for Gemini, so the caller substitutes it.
		if endsWithVersion(version) {
			return "models/{model}:generateContent"
		}
		return version + "/models/{model}:generateContent"
	default:
		// OpenAI-wire vendors: the configured endpoint already carries /v1.
		if endsWithVersion("v1") {
			return "chat/completions"
		}
		return "v1/chat/completions"
	}
}

// ModelString returns the model value for a surface. The unified router needs
// "{slug}/{model}"; every other surface takes the bare model name.
func (h *harness) ModelString(surface vc.Surface, v vc.VendorConfig, model string) string {
	if surface == vc.SurfaceUniversal {
		return v.Slug() + "/" + model
	}
	return model
}

// post sends a request to the gateway and reads the whole response, retrying
// through upstream rate limits so a busy account does not masquerade as a
// product failure.
func (h *harness) post(ctx context.Context, url string, body []byte, extraHeaders map[string]string) (*response, error) {
	var last *response
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := h.attemptContext(ctx)
		resp, err := h.postOnce(attemptCtx, url, body, extraHeaders)
		cancel()
		if err != nil {
			return nil, err
		}
		last = resp

		limited, wait := isRateLimited(resp.Status, resp.Body)
		if !limited || attempt >= maxRateLimitRetries {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, nil
		case <-time.After(wait):
		}
	}
}

func (h *harness) postOnce(ctx context.Context, url string, body []byte, extraHeaders map[string]string) (*response, error) {
	release := h.acquire()
	defer release()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return &response{Status: resp.StatusCode, Body: data, Header: resp.Header}, nil
}

// postStream sends a request and parses the SSE response without buffering the
// whole body first, so a stream that never terminates fails on the context
// deadline rather than hanging.
func (h *harness) postStream(ctx context.Context, url string, body []byte, extraHeaders map[string]string) (*vc.Stream, *response, error) {
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := h.attemptContext(ctx)
		stream, meta, err := h.postStreamOnce(attemptCtx, url, body, extraHeaders)
		cancel()
		if err != nil {
			return nil, meta, err
		}

		// A rate limit during streaming arrives as a 200 whose first frame is an
		// error envelope, because the headers were already flushed. Reading the
		// first frame is the only way to see it.
		probe := meta.Body
		if probe == nil && stream != nil && len(stream.DataFrames()) > 0 {
			probe = stream.DataFrames()[0].Raw
		}
		limited, wait := isRateLimited(meta.Status, probe)
		if !limited || attempt >= maxRateLimitRetries {
			return stream, meta, nil
		}
		select {
		case <-ctx.Done():
			return stream, meta, nil
		case <-time.After(wait):
		}
	}
}

func (h *harness) postStreamOnce(ctx context.Context, url string, body []byte, extraHeaders map[string]string) (*vc.Stream, *response, error) {
	release := h.acquire()
	defer release()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	meta := &response{Status: resp.StatusCode, Header: resp.Header}
	if resp.StatusCode != http.StatusOK {
		meta.Body, _ = io.ReadAll(resp.Body)
		return nil, meta, nil
	}

	// A 200 that is not an event stream means the gateway answered a streaming
	// request with a buffered response. Parsing it as SSE would yield zero
	// frames and a pile of downstream assertion failures with no evidence
	// attached, so capture the body instead — it is the finding.
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		meta.Body, _ = io.ReadAll(resp.Body)
		return nil, meta, nil
	}

	stream, err := vc.ParseStream(resp.Body)
	if err != nil {
		return nil, meta, err
	}
	return stream, meta, nil
}

// streamCarriedUpstreamError reports the error envelope a stream delivered
// instead of content, if any. A 200 carrying only an error object is an
// upstream failure, and reporting it as a pile of schema violations would bury
// the actual cause.
func streamCarriedUpstreamError(s *vc.Stream) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	frames := s.DataFrames()
	if len(frames) == 0 {
		return nil, false
	}
	if looksLikeErrorEnvelope(frames[0].Raw) {
		return frames[0].Raw, true
	}
	return nil, false
}

// vendorHeaders returns the extra headers a vendor's native API requires on the
// sdk surface. On the shim and universal surfaces the translator supplies them.
func vendorHeaders(surface vc.Surface, v vc.VendorConfig) map[string]string {
	if surface != vc.SurfaceSDK {
		return nil
	}
	headers := map[string]string{}
	if v.Vendor == "anthropic" {
		version := v.Extra["api_version"]
		if version == "" {
			version = "2023-06-01"
		}
		headers["anthropic-version"] = version
		if beta := v.Extra["beta_headers"]; beta != "" {
			headers["anthropic-beta"] = beta
		}
	}
	return headers
}

// responseFormat is the wire format a surface returns for a vendor. The shim and
// universal surfaces always speak OpenAI; the sdk surface speaks the vendor's
// own dialect, which is the entire reason it is tested separately.
func responseFormat(surface vc.Surface, v vc.VendorConfig) vc.Format {
	if surface != vc.SurfaceSDK {
		return vc.FormatOpenAICompletion
	}
	switch v.Vendor {
	case "anthropic":
		return vc.FormatAnthropicMessage
	case "google_ai":
		return vc.FormatGeminiGenerate
	default:
		return vc.FormatOpenAICompletion
	}
}

// streamFormat is the wire format of an SSE frame. It is a separate decision
// from responseFormat because streaming dialects diverge further than buffered
// ones: Anthropic's native stream is a typed event state machine, nothing like
// its buffered message object.
//
// The second return reports whether the stream follows OpenAI's chunk
// conventions ([DONE], delta.role), which the native dialects do not.
func streamFormat(surface vc.Surface, v vc.VendorConfig) (vc.Format, bool) {
	if surface != vc.SurfaceSDK {
		return vc.FormatOpenAIChunk, true
	}
	switch v.Vendor {
	case "anthropic":
		return vc.FormatAnthropicStreamEvent, false
	default:
		return vc.FormatOpenAIChunk, true
	}
}

// bodyExcerpt renders a response body for a failure message, truncated so a
// multi-megabyte error page does not bury the assertion.
func bodyExcerpt(b []byte) string {
	const max = 2000
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(empty body)"
	}
	if len(s) > max {
		return s[:max] + fmt.Sprintf("... (%d bytes total)", len(s))
	}
	return s
}

// readAll drains a reader, failing the test rather than returning an error so
// callers stay readable.
func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(r)
	require.NoError(t, err, "reading body")
	return data
}

// requireJSON fails the test with the raw body when a response is not JSON,
// which is the common shape of an upstream error page or a proxy panic.
func requireJSON(t *testing.T, body []byte) {
	t.Helper()
	var probe any
	require.NoErrorf(t, json.Unmarshal(body, &probe),
		"response is not valid JSON:\n%s", bodyExcerpt(body))
}
