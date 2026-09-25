package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
)

// MockProfiles are the upstream behaviours the benchmark LLMs point at. The
// profile rides in the LLM's base URL (/p/{spec}/v1), so it applies on every
// gateway path. Scenario files refer to them as {mock:<name>}.
var MockProfiles = map[string]string{
	// Answers immediately: isolates gateway cost.
	"fast": "tokens=16",
	// A quick upstream: the throughput-ceiling scenario.
	"20ms": "ttft=20ms,tokens=16",
	// Shaped like a hosted chat model: long-lived streams, many in flight.
	"realistic": "ttft=300ms,ttft_p99=900ms,tps=50,tokens=200",
	// Many small chunks back to back: per-chunk relay cost.
	"long": "tokens=1000,token_text=xx",
}

// MockModel is the model name mock requests use (prices are seeded for it).
const MockModel = "mock-model"

// The minimal-configuration target (seed -minimal): an app with no budget and
// an LLM on the instant mock whose model has no price, so the edge skips the
// budget reads and, with cost zero, the budget-usage writes. It is what
// configuration alone can take off the request path; authentication and the
// analytics insert still run.
// benchBudget is the main app's monthly budget: large enough never to block,
// so the edge's budget check does its full work on every request.
const benchBudget = 1_000_000.0

const (
	MinimalLLM   = "min-instant"
	MinimalModel = "mock-unpriced"
	MinimalApp   = "gateway-benchmark-minimal"
)

// Config is what Run needs.
type Config struct {
	StudioURL string
	Email     string
	Password  string
	// MockUpstreamURL is the mock's base URL as the *gateway* reaches it
	// (e.g. http://mockllm:9999 inside compose).
	MockUpstreamURL string
	// Vendors adds real-vendor LLMs from test-secrets/vendors.env when true.
	Vendors bool
	// Minimal adds the minimal-configuration app and LLM (see MinimalApp).
	Minimal   bool
	Namespace string
	MinEdges  int
	// GatewayURL is the edge gateway as the load generator reaches it; used
	// for a readiness request through every seeded route.
	GatewayURL  string
	SyncTimeout time.Duration
}

// State is written after seeding and read by every run. It holds the app
// secret, so it lives in a gitignored directory.
type State struct {
	SeededAt  time.Time         `json:"seeded_at"`
	StudioURL string            `json:"studio_url"`
	AppID     int               `json:"app_id"`
	AppSecret string            `json:"app_secret"`
	APIKey    string            `json:"studio_api_key"`
	LLMs      map[string]string `json:"llms"` // route slug -> vendor
	Models    map[string]string `json:"models"`
	Checksum  string            `json:"config_checksum"`
	// MinimalAppSecret is set when seeded with Minimal.
	MinimalAppID     int    `json:"minimal_app_id,omitempty"`
	MinimalAppSecret string `json:"minimal_app_secret,omitempty"`
}

// Run seeds Studio and waits for the edges, then proves every route answers.
func Run(ctx context.Context, cfg Config, log func(string, ...any)) (*State, error) {
	s := NewStudio(cfg.StudioURL)
	if err := s.Login(ctx, cfg.Email, cfg.Password); err != nil {
		return nil, err
	}
	log("logged in to Studio as %s", cfg.Email)
	userID, err := s.MyUserID(ctx)
	if err != nil {
		return nil, err
	}

	var llms []LLMSpec
	mock := strings.TrimSuffix(cfg.MockUpstreamURL, "/")
	for name, spec := range MockProfiles {
		llms = append(llms, LLMSpec{
			Name: "mock-" + name, Vendor: "openai", Endpoint: mock + "/p/" + url.PathEscape(spec) + "/v1",
			APIKey: "mock-key", DefaultModel: MockModel,
		})
	}
	llms = append(llms, LLMSpec{
		Name: "mock-anthropic", Vendor: "anthropic", Endpoint: mock + "/p/" + url.PathEscape(MockProfiles["fast"]) + "/v1",
		APIKey: "mock-key", DefaultModel: MockModel,
	})

	models := map[string]string{"mock": MockModel}
	if cfg.Vendors {
		vc, err := vendorconformance.Load()
		if err != nil {
			return nil, fmt.Errorf("loading vendor credentials: %w", err)
		}
		for _, key := range []string{"openai", "anthropic"} {
			v, ok := vc.Vendor(key)
			if !ok {
				log("vendor %s not configured in %s; skipping it", key, vendorconformance.DefaultEnvFile)
				continue
			}
			model := v.Models[vendorconformance.ModelLatest]
			// Prefixed: Studio ships default LLMs whose slugs are the bare
			// vendor names, and a duplicate slug makes the edge reject the
			// whole config snapshot.
			llms = append(llms, LLMSpec{Name: "bench-" + key, Vendor: string(v.Vendor), Endpoint: v.Endpoint, APIKey: v.APIKey, DefaultModel: model})
			models[key] = model
		}
	}

	// Prices go in before any traffic: the first request for an unpriced
	// model auto-creates a zero-cost row that later blocks a real one.
	for key, model := range models {
		vendors := []string{key}
		if key == "mock" {
			vendors = []string{"openai", "anthropic"}
		}
		for _, v := range vendors {
			// Nominal per-token prices, so cost and budget code does real work.
			if err := s.EnsurePrice(ctx, model, v, 0.00001, 0.000002); err != nil {
				return nil, err
			}
		}
	}

	state := &State{SeededAt: time.Now().UTC(), StudioURL: cfg.StudioURL, APIKey: s.apiKey,
		LLMs: map[string]string{}, Models: models}
	var ids []int
	for _, l := range llms {
		id, err := s.EnsureLLM(ctx, l)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		state.LLMs[l.Name] = l.Vendor
		log("LLM %-16s -> %s", l.Name, redactEndpoint(l.Endpoint))
	}

	app, err := s.EnsureApp(ctx, "gateway-benchmark", userID, ids, benchBudget)
	if err != nil {
		return nil, err
	}
	state.AppID, state.AppSecret = app.ID, app.Secret
	log("app %d has an active credential", app.ID)

	if cfg.Minimal {
		spec := LLMSpec{Name: MinimalLLM, Vendor: "openai", Endpoint: mock + "/p/" + url.PathEscape(MockProfiles["fast"]) + "/v1",
			APIKey: "mock-key", DefaultModel: MinimalModel}
		id, err := s.EnsureLLM(ctx, spec)
		if err != nil {
			return nil, err
		}
		minApp, err := s.EnsureApp(ctx, MinimalApp, userID, []int{id}, 0)
		if err != nil {
			return nil, err
		}
		state.MinimalAppID, state.MinimalAppSecret = minApp.ID, minApp.Secret
		log("minimal configuration: LLM %s (model %s, no price), app %d without a budget", MinimalLLM, MinimalModel, minApp.ID)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}
	st, err := s.PushAndWaitForEdges(ctx, ns, cfg.MinEdges, cfg.SyncTimeout)
	if err != nil {
		return nil, err
	}
	state.Checksum = st.Namespace.ExpectedChecksum
	log("%d/%d edge(s) in sync at checksum %.12s", st.Namespace.SyncedCount, st.Namespace.TotalEdges, st.Namespace.ExpectedChecksum)

	if cfg.GatewayURL != "" {
		if err := waitForRoutes(ctx, cfg.GatewayURL, app.Secret, state, 2*time.Minute); err != nil {
			return nil, err
		}
		if state.MinimalAppSecret != "" {
			minimal := &State{LLMs: map[string]string{MinimalLLM: "openai"}, Models: map[string]string{"mock": MinimalModel}}
			if err := waitForRoutes(ctx, cfg.GatewayURL, state.MinimalAppSecret, minimal, 2*time.Minute); err != nil {
				return nil, err
			}
		}
		log("every seeded route answers through the gateway")
	}
	return state, nil
}

// waitForRoutes sends one request per seeded LLM through the gateway until all
// return 200: the edge has the config and the credential validates.
func waitForRoutes(ctx context.Context, gatewayURL, secret string, st *State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 60 * time.Second}
	for slug, vendor := range st.LLMs {
		model := st.Models["mock"]
		path := "/llm/rest/" + slug + "/v1/chat/completions"
		body := `{"model":"%s","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`
		if vendor == "anthropic" {
			path = "/llm/rest/" + slug + "/v1/messages"
		}
		if key, ok := strings.CutPrefix(slug, "bench-"); ok {
			// Real vendors: a single tiny request confirms the key works.
			model = st.Models[key]
			if key == "openai" {
				body = `{"model":"%s","messages":[{"role":"user","content":"ping"}],"max_completion_tokens":16}`
			}
		}
		payload := fmt.Sprintf(body, model)
		for {
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(gatewayURL, "/")+path, strings.NewReader(payload))
			req.Header.Set("Authorization", "Bearer "+secret)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			resp, err := client.Do(req)
			status := 0
			var respBody []byte
			if err == nil {
				status = resp.StatusCode
				respBody = make([]byte, 512)
				n, _ := resp.Body.Read(respBody)
				respBody = respBody[:n]
				resp.Body.Close()
			}
			if status == http.StatusOK {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("route %s not ready through the gateway: status=%d err=%v body=%s", slug, status, err, respBody)
			}
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}

// WriteState saves the state file (0600: it holds credentials).
func WriteState(path string, st *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// ReadState loads a state file written by WriteState.
func ReadState(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s (run `gwbench seed` first): %w", path, err)
	}
	var st State
	return &st, json.Unmarshal(b, &st)
}

// redactEndpoint returns e fit for logs: user information and the query
// string, the two places a URL carries credentials, are replaced.
func redactEndpoint(e string) string {
	u, err := url.Parse(e)
	if err != nil {
		return "[unparseable endpoint]"
	}
	if u.User != nil {
		u.User = url.User("***")
	}
	if u.RawQuery != "" {
		u.RawQuery = "***"
	}
	return u.String()
}
