// Package vendorconformance holds the shared, module-neutral half of the live
// vendor conformance suite: configuration loading, the vendor/surface/scenario
// support matrix, canonical wire schemas, response normalization and drift
// detection.
//
// It deliberately imports nothing from either module's internal packages so
// that both the root module (Studio chat tests) and the microgateway module
// (gateway ingress tests) can use it. The harnesses that actually boot a server
// live next to the code they boot:
//
//	microgateway/tests/vendorconformance/  - shim, sdk, universal surfaces
//	tests/vendorconformance/               - chat surface
//
// Nothing here reads the network or asserts anything; it is all pure config and
// data so it can be unit-tested without credentials.
package vendorconformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/joho/godotenv"
)

// DefaultEnvFile is the credentials file, relative to the repository root.
const DefaultEnvFile = "test-secrets/vendors.env"

// Surface is one ingress path into the platform. Each exercises a materially
// different translation code path — see features/VendorConformance.md §1.1.
type Surface string

const (
	// SurfaceChat is the Studio chat session: langchaingo drivers, not the proxy.
	SurfaceChat Surface = "chat"
	// SurfaceShim is /ai/{slug}/v1/chat/completions — OpenAI-format translation.
	SurfaceShim Surface = "shim"
	// SurfaceSDK is /llm/call/{slug}/... — native vendor passthrough.
	SurfaceSDK Surface = "sdk"
	// SurfaceUniversal is /v1/chat/completions with a "{slug}/{model}" model string.
	SurfaceUniversal Surface = "universal"
)

// AllSurfaces is the default surface set, in the order they should be reported.
var AllSurfaces = []Surface{SurfaceChat, SurfaceShim, SurfaceSDK, SurfaceUniversal}

// Scenario is one behaviour exercised on a (vendor, surface, model) triple.
type Scenario string

const (
	ScenarioBasic          Scenario = "basic"
	ScenarioStreaming      Scenario = "streaming"
	ScenarioMultiturn      Scenario = "multiturn"
	ScenarioSystem         Scenario = "system"
	ScenarioTools          Scenario = "tools"
	ScenarioToolsStreaming Scenario = "tools_streaming"
	ScenarioJSONMode       Scenario = "json_mode"
	ScenarioVision         Scenario = "vision"
	ScenarioLongContext    Scenario = "long_context"
	ScenarioErrors         Scenario = "errors"
	ScenarioUsage          Scenario = "usage"
)

// AllScenarios is the default scenario set, ordered cheapest-and-most-diagnostic
// first so an early failure is the most informative one.
var AllScenarios = []Scenario{
	ScenarioBasic, ScenarioStreaming, ScenarioUsage, ScenarioSystem,
	ScenarioMultiturn, ScenarioTools, ScenarioToolsStreaming,
	ScenarioJSONMode, ScenarioVision, ScenarioLongContext, ScenarioErrors,
}

// ModelSlot names one of the model IDs configured per vendor.
type ModelSlot string

const (
	// ModelLatest is the current generation model family.
	ModelLatest ModelSlot = "latest"
	// ModelPrev is the latest-1 generation model family.
	ModelPrev ModelSlot = "prev"
	// ModelReasoning is an optional reasoning-family model whose usage object
	// carries extra counters and which rejects some sampling parameters.
	ModelReasoning ModelSlot = "reasoning"
	// ModelVision is an optional multimodal model.
	ModelVision ModelSlot = "vision"
)

// VendorConfig is one fully-resolved, credentialed vendor ready to be seeded as
// an LLM row. Only vendors that passed validation appear in Config.Vendors.
type VendorConfig struct {
	// Key is the stable identifier used in env var names, route slugs and
	// artifact filenames (e.g. "openai"). Unique within a run.
	Key string
	// Vendor is the platform vendor enum this config is seeded as.
	Vendor models.Vendor
	// APIKey is the credential stored on LLM.APIKey. Empty for vendors that
	// authenticate out of band (bedrock via Metadata, vertex via ADC).
	APIKey string
	// Endpoint is stored verbatim on LLM.APIEndpoint. Its meaning is
	// vendor-specific: a base URL for most, "project:location" for vertex, a
	// region-bearing URL for bedrock.
	Endpoint string
	// Models maps each configured slot to a model ID. ModelLatest is always
	// present; the rest are optional.
	Models map[ModelSlot]string
	// Metadata is copied onto LLM.Metadata (bedrock AWS credentials).
	Metadata map[string]string
	// Extra carries non-secret per-vendor knobs used when building requests
	// (API versions, beta headers) rather than when seeding the LLM row.
	Extra map[string]string
	// EnvPrefix is the variable-name prefix this vendor was loaded from, e.g.
	// "VT_GOOGLEAI". Error messages quote real variable names with it; deriving
	// one from Key would produce "VT_GOOGLE_AI", which does not exist.
	EnvPrefix string
}

// EnvVar names one of this vendor's configuration variables.
func (v VendorConfig) EnvVar(suffix string) string {
	return v.EnvPrefix + "_" + suffix
}

// ModelEnvVar names the variable holding a model slot's ID.
func (v VendorConfig) ModelEnvVar(slot ModelSlot) string {
	return v.EnvVar("MODEL_" + strings.ToUpper(string(slot)))
}

// Model returns the model ID for a slot, and whether it was configured.
func (v VendorConfig) Model(slot ModelSlot) (string, bool) {
	m, ok := v.Models[slot]
	return m, ok && m != ""
}

// ModelSlots returns the slots to exercise for this vendor, honouring
// Config.IncludePrevModel. Always at least ModelLatest.
func (v VendorConfig) ModelSlots(includePrev bool) []ModelSlot {
	slots := []ModelSlot{ModelLatest}
	if includePrev {
		if _, ok := v.Model(ModelPrev); ok {
			slots = append(slots, ModelPrev)
		}
	}
	return slots
}

// Slug is the route slug this vendor is seeded under. It is also the vendor
// prefix in a unified-router "{slug}/{model}" model string, so it must satisfy
// proxy.unifiedRouteSlugPattern (`^[A-Za-z0-9_-]+$`).
func (v VendorConfig) Slug() string {
	return "vt-" + strings.ReplaceAll(v.Key, "_", "-")
}

// SkippedVendor records a vendor that was not runnable, and why. Skips are
// reported rather than swallowed: a silent skip is how a vendor quietly stops
// being tested.
type SkippedVendor struct {
	Key    string
	Reason string
}

// Config is the fully-resolved suite configuration.
type Config struct {
	Enabled bool

	Vendors []VendorConfig
	Skipped []SkippedVendor

	Surfaces  []Surface
	Scenarios []Scenario

	IncludePrevModel    bool
	StrictUnknownFields bool
	UpdateGolden        bool

	MaxTokens int
	Timeout   time.Duration
	Parallel  int

	ArtifactDir string
	RepoRoot    string

	// Live-stack mode (not yet implemented). Blank means in-process boot, which
	// is the supported mode; see features/VendorConformance.md §4.
	StudioBaseURL       string
	MicrogatewayBaseURL string
	APIToken            string
}

// HasSurface reports whether a surface is in the configured set.
func (c *Config) HasSurface(s Surface) bool {
	for _, x := range c.Surfaces {
		if x == s {
			return true
		}
	}
	return false
}

// HasScenario reports whether a scenario is in the configured set.
func (c *Config) HasScenario(s Scenario) bool {
	for _, x := range c.Scenarios {
		if x == s {
			return true
		}
	}
	return false
}

// Vendor returns the configured vendor with the given key.
func (c *Config) Vendor(key string) (VendorConfig, bool) {
	for _, v := range c.Vendors {
		if v.Key == key {
			return v, true
		}
	}
	return VendorConfig{}, false
}

// Load reads the credentials file and the process environment into a Config.
//
// Process environment wins over file values (godotenv.Load never overwrites an
// already-set variable), so CI can inject secrets with no file on disk.
//
// A returned error means the suite is misconfigured and should fail. A vendor
// that is merely missing credentials is not an error: it lands in Skipped.
func Load() (*Config, error) {
	root, err := RepoRoot()
	if err != nil {
		return nil, err
	}

	envFile := os.Getenv("VENDOR_TESTS_ENV_FILE")
	if envFile == "" {
		envFile = filepath.Join(root, DefaultEnvFile)
	}
	if _, statErr := os.Stat(envFile); statErr == nil {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("loading %s: %w", envFile, err)
		}
	} else if os.Getenv("VENDOR_TESTS_ENV_FILE") != "" {
		// An explicitly named file that does not exist is a mistake worth
		// surfacing; a missing default file just means "use the environment".
		return nil, fmt.Errorf("VENDOR_TESTS_ENV_FILE=%s does not exist", envFile)
	}

	cfg := &Config{
		Enabled:             envBool("VENDOR_TESTS_ENABLED", false),
		IncludePrevModel:    envBool("VENDOR_TESTS_INCLUDE_PREV_MODEL", true),
		StrictUnknownFields: envBool("VENDOR_TESTS_STRICT_UNKNOWN_FIELDS", false),
		UpdateGolden:        envBool("VENDOR_TESTS_UPDATE_GOLDEN", false),
		MaxTokens:           envInt("VENDOR_TESTS_MAX_TOKENS", 256),
		Parallel:            envInt("VENDOR_TESTS_PARALLEL", 4),
		RepoRoot:            root,
		StudioBaseURL:       strings.TrimSpace(os.Getenv("VENDOR_TESTS_STUDIO_BASE_URL")),
		MicrogatewayBaseURL: strings.TrimSpace(os.Getenv("VENDOR_TESTS_MICROGATEWAY_BASE_URL")),
		APIToken:            strings.TrimSpace(os.Getenv("VENDOR_TESTS_API_TOKEN")),
	}

	timeout, err := envDuration("VENDOR_TESTS_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, err
	}
	cfg.Timeout = timeout

	cfg.ArtifactDir = strings.TrimSpace(os.Getenv("VENDOR_TESTS_ARTIFACT_DIR"))
	if cfg.ArtifactDir == "" {
		cfg.ArtifactDir = filepath.Join(root, "test-results", "vendor-conformance")
	} else if !filepath.IsAbs(cfg.ArtifactDir) {
		cfg.ArtifactDir = filepath.Join(root, cfg.ArtifactDir)
	}

	if cfg.Surfaces, err = parseSurfaces(os.Getenv("VENDOR_TESTS_SURFACES")); err != nil {
		return nil, err
	}
	if cfg.Scenarios, err = parseScenarios(os.Getenv("VENDOR_TESTS_SCENARIOS")); err != nil {
		return nil, err
	}

	filter, err := parseVendorFilter(os.Getenv("VENDOR_TESTS_VENDORS"))
	if err != nil {
		return nil, err
	}

	for _, loader := range vendorLoaders() {
		if filter != nil && !filter[loader.key] {
			continue
		}
		vc, skip := loader.load()
		if skip != "" {
			cfg.Skipped = append(cfg.Skipped, SkippedVendor{Key: loader.key, Reason: skip})
			continue
		}
		cfg.Vendors = append(cfg.Vendors, vc)
	}

	cfg.Vendors = append(cfg.Vendors, loadCompatVendors(filter, &cfg.Skipped)...)

	sort.Slice(cfg.Skipped, func(i, j int) bool { return cfg.Skipped[i].Key < cfg.Skipped[j].Key })
	return cfg, nil
}

// vendorLoader pairs a vendor key with the function that resolves it from the
// environment. Each loader returns either a config or a non-empty skip reason.
type vendorLoader struct {
	key  string
	load func() (VendorConfig, string)
}

func vendorLoaders() []vendorLoader {
	return []vendorLoader{
		{"openai", loadOpenAI},
		{"anthropic", loadAnthropic},
		{"google_ai", loadGoogleAI},
		{"vertex", loadVertex},
		{"bedrock", loadBedrock},
		{"ollama", loadOllama},
		{"huggingface", loadHuggingFace},
	}
}

func loadOpenAI() (VendorConfig, string) {
	key := env("VT_OPENAI_API_KEY")
	model := env("VT_OPENAI_MODEL_LATEST")
	if key == "" {
		return VendorConfig{}, "VT_OPENAI_API_KEY not set"
	}
	if model == "" {
		return VendorConfig{}, "VT_OPENAI_MODEL_LATEST not set"
	}
	return VendorConfig{
		Key:       "openai",
		EnvPrefix: "VT_OPENAI",
		Vendor:    models.OPENAI,
		APIKey:    key,
		Endpoint:  envOr("VT_OPENAI_ENDPOINT", "https://api.openai.com/v1"),
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest:    model,
			ModelPrev:      env("VT_OPENAI_MODEL_PREV"),
			ModelReasoning: env("VT_OPENAI_MODEL_REASONING"),
			ModelVision:    env("VT_OPENAI_MODEL_VISION"),
		}),
	}, ""
}

func loadAnthropic() (VendorConfig, string) {
	key := env("VT_ANTHROPIC_API_KEY")
	model := env("VT_ANTHROPIC_MODEL_LATEST")
	if key == "" {
		return VendorConfig{}, "VT_ANTHROPIC_API_KEY not set"
	}
	if model == "" {
		return VendorConfig{}, "VT_ANTHROPIC_MODEL_LATEST not set"
	}
	return VendorConfig{
		Key:       "anthropic",
		EnvPrefix: "VT_ANTHROPIC",
		Vendor:    models.ANTHROPIC,
		APIKey:    key,
		Endpoint:  envOr("VT_ANTHROPIC_ENDPOINT", "https://api.anthropic.com"),
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_ANTHROPIC_MODEL_PREV"),
		}),
		Extra: nonEmptyStr(map[string]string{
			"api_version":  envOr("VT_ANTHROPIC_API_VERSION", "2023-06-01"),
			"beta_headers": env("VT_ANTHROPIC_BETA_HEADERS"),
		}),
	}, ""
}

func loadGoogleAI() (VendorConfig, string) {
	key := env("VT_GOOGLEAI_API_KEY")
	model := env("VT_GOOGLEAI_MODEL_LATEST")
	if key == "" {
		return VendorConfig{}, "VT_GOOGLEAI_API_KEY not set"
	}
	if model == "" {
		return VendorConfig{}, "VT_GOOGLEAI_MODEL_LATEST not set"
	}
	return VendorConfig{
		Key:       "google_ai",
		EnvPrefix: "VT_GOOGLEAI",
		Vendor:    models.GOOGLEAI,
		APIKey:    key,
		Endpoint:  envOr("VT_GOOGLEAI_ENDPOINT", "https://generativelanguage.googleapis.com"),
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_GOOGLEAI_MODEL_PREV"),
		}),
		Extra: map[string]string{"api_version": envOr("VT_GOOGLEAI_API_VERSION", "v1beta")},
	}, ""
}

func loadVertex() (VendorConfig, string) {
	project := env("VT_VERTEX_PROJECT")
	creds := env("VT_VERTEX_CREDENTIALS_FILE")
	model := env("VT_VERTEX_MODEL_LATEST")
	switch {
	case project == "":
		return VendorConfig{}, "VT_VERTEX_PROJECT not set"
	case creds == "":
		return VendorConfig{}, "VT_VERTEX_CREDENTIALS_FILE not set"
	case model == "":
		return VendorConfig{}, "VT_VERTEX_MODEL_LATEST not set"
	}
	if _, err := os.Stat(creds); err != nil {
		return VendorConfig{}, fmt.Sprintf("VT_VERTEX_CREDENTIALS_FILE %s is not readable: %v", creds, err)
	}
	location := envOr("VT_VERTEX_LOCATION", "us-central1")
	return VendorConfig{
		Key:       "vertex",
		EnvPrefix: "VT_VERTEX",
		Vendor:    models.VERTEX,
		// vendors/vertex/vertex.go splits APIEndpoint on ":" into project and
		// location. It is not a URL.
		Endpoint: project + ":" + location,
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_VERTEX_MODEL_PREV"),
		}),
		Extra: map[string]string{
			"credentials_file": creds,
			"project":          project,
			"location":         location,
		},
	}, ""
}

func loadBedrock() (VendorConfig, string) {
	accessKey := env("VT_BEDROCK_ACCESS_KEY_ID")
	secretKey := env("VT_BEDROCK_SECRET_ACCESS_KEY")
	model := env("VT_BEDROCK_MODEL_LATEST")
	switch {
	case accessKey == "":
		return VendorConfig{}, "VT_BEDROCK_ACCESS_KEY_ID not set"
	case secretKey == "":
		return VendorConfig{}, "VT_BEDROCK_SECRET_ACCESS_KEY not set"
	case model == "":
		return VendorConfig{}, "VT_BEDROCK_MODEL_LATEST not set"
	}

	region := env("VT_BEDROCK_REGION")
	endpoint := env("VT_BEDROCK_ENDPOINT")
	if endpoint == "" {
		if region == "" {
			return VendorConfig{}, "one of VT_BEDROCK_REGION or VT_BEDROCK_ENDPOINT must be set"
		}
		endpoint = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", region)
	} else if region != "" {
		// The vendor client derives the signing region from the endpoint
		// hostname and ignores any separately configured region. Rather than
		// let a contradictory VT_BEDROCK_REGION look effective, refuse it.
		if epRegion, err := regionFromBedrockEndpoint(endpoint); err != nil {
			return VendorConfig{}, fmt.Sprintf("VT_BEDROCK_ENDPOINT %q: %v", endpoint, err)
		} else if epRegion != region {
			return VendorConfig{}, fmt.Sprintf(
				"VT_BEDROCK_REGION=%s contradicts VT_BEDROCK_ENDPOINT (%s implies region %s); "+
					"the vendor client signs with the endpoint's region, so set them to match or clear VT_BEDROCK_REGION",
				region, endpoint, epRegion)
		}
	}

	return VendorConfig{
		Key:       "bedrock",
		EnvPrefix: "VT_BEDROCK",
		Vendor:    models.BEDROCK,
		Endpoint:  endpoint,
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_BEDROCK_MODEL_PREV"),
		}),
		// vendors/bedrock/bedrock.go reads credentials from LLM.Metadata under
		// exactly these keys.
		Metadata: nonEmptyStr(map[string]string{
			"aws_access_key_id":     accessKey,
			"aws_secret_access_key": secretKey,
			"aws_session_token":     env("VT_BEDROCK_SESSION_TOKEN"),
		}),
		Extra: nonEmptyStr(map[string]string{
			"anthropic_bridge_model": env("VT_BEDROCK_ANTHROPIC_MODEL"),
		}),
	}, ""
}

func loadOllama() (VendorConfig, string) {
	endpoint := env("VT_OLLAMA_ENDPOINT")
	model := env("VT_OLLAMA_MODEL_LATEST")
	if endpoint == "" {
		return VendorConfig{}, "VT_OLLAMA_ENDPOINT not set"
	}
	if model == "" {
		return VendorConfig{}, "VT_OLLAMA_MODEL_LATEST not set"
	}
	return VendorConfig{
		Key:       "ollama",
		EnvPrefix: "VT_OLLAMA",
		Vendor:    models.OLLAMA,
		Endpoint:  endpoint,
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_OLLAMA_MODEL_PREV"),
		}),
	}, ""
}

func loadHuggingFace() (VendorConfig, string) {
	key := env("VT_HUGGINGFACE_API_KEY")
	endpoint := env("VT_HUGGINGFACE_ENDPOINT")
	model := env("VT_HUGGINGFACE_MODEL_LATEST")
	switch {
	case key == "":
		return VendorConfig{}, "VT_HUGGINGFACE_API_KEY not set"
	case endpoint == "":
		return VendorConfig{}, "VT_HUGGINGFACE_ENDPOINT not set"
	case model == "":
		return VendorConfig{}, "VT_HUGGINGFACE_MODEL_LATEST not set"
	}
	return VendorConfig{
		Key:       "huggingface",
		EnvPrefix: "VT_HUGGINGFACE",
		Vendor:    models.HUGGINGFACE,
		APIKey:    key,
		Endpoint:  endpoint,
		Models: nonEmpty(map[ModelSlot]string{
			ModelLatest: model,
			ModelPrev:   env("VT_HUGGINGFACE_MODEL_PREV"),
		}),
	}, ""
}

// loadCompatVendors scans VT_COMPAT_{n}_* slots until it finds a blank NAME.
// Each is registered as an OpenAI-wire vendor with a custom endpoint.
func loadCompatVendors(filter map[string]bool, skipped *[]SkippedVendor) []VendorConfig {
	var out []VendorConfig
	for i := 1; ; i++ {
		name := env(fmt.Sprintf("VT_COMPAT_%d_NAME", i))
		if name == "" {
			return out
		}
		key := "compat_" + strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(name))
		if filter != nil && !filter[key] {
			continue
		}
		endpoint := env(fmt.Sprintf("VT_COMPAT_%d_ENDPOINT", i))
		model := env(fmt.Sprintf("VT_COMPAT_%d_MODEL_LATEST", i))
		if endpoint == "" || model == "" {
			*skipped = append(*skipped, SkippedVendor{
				Key:    key,
				Reason: fmt.Sprintf("VT_COMPAT_%d_ENDPOINT or VT_COMPAT_%d_MODEL_LATEST not set", i, i),
			})
			continue
		}
		out = append(out, VendorConfig{
			Key:       key,
			EnvPrefix: fmt.Sprintf("VT_COMPAT_%d", i),
			Vendor:    models.OPENAI,
			APIKey:    env(fmt.Sprintf("VT_COMPAT_%d_API_KEY", i)),
			Endpoint:  endpoint,
			Models: nonEmpty(map[ModelSlot]string{
				ModelLatest: model,
				ModelPrev:   env(fmt.Sprintf("VT_COMPAT_%d_MODEL_PREV", i)),
			}),
			Extra: map[string]string{"display_name": name},
		})
	}
}

// regionFromBedrockEndpoint mirrors vendors/bedrock.ParseRegionFromEndpoint.
// It is duplicated rather than imported so this package stays free of vendor
// dependencies; TestRegionFromBedrockEndpointMatchesVendor pins them together.
func regionFromBedrockEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("bedrock endpoint is empty")
	}
	if !strings.Contains(endpoint, ".") && !strings.Contains(endpoint, "/") {
		return endpoint, nil
	}
	host := endpoint
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i+1:], ".") {
		host = host[:i] // strip port
	}
	if host == "" {
		return "", fmt.Errorf("invalid bedrock endpoint URL")
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && (parts[0] == "bedrock-runtime" || parts[0] == "bedrock-mantle") {
		return parts[1], nil
	}
	return "", fmt.Errorf("cannot extract region from endpoint (expected bedrock-runtime.{region}.amazonaws.com or bedrock-mantle.{region}.api.aws)")
}

// RepoRoot walks up from the working directory to the directory holding the
// root go.mod. Tests run from their own package directory, in either module, so
// no relative path to test-secrets/ would be correct for all of them.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/TykTechnologies/midsommar/v2") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no go.mod for module .../midsommar/v2 above %s)", must(os.Getwd()))
		}
		dir = parent
	}
}

func parseSurfaces(raw string) ([]Surface, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]Surface(nil), AllSurfaces...), nil
	}
	valid := map[Surface]bool{}
	for _, s := range AllSurfaces {
		valid[s] = true
	}
	var out []Surface
	for _, item := range splitList(raw) {
		s := Surface(item)
		if !valid[s] {
			return nil, fmt.Errorf("VENDOR_TESTS_SURFACES: unknown surface %q (valid: %s)", item, joinSurfaces(AllSurfaces))
		}
		out = append(out, s)
	}
	return out, nil
}

func parseScenarios(raw string) ([]Scenario, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]Scenario(nil), AllScenarios...), nil
	}
	valid := map[Scenario]bool{}
	for _, s := range AllScenarios {
		valid[s] = true
	}
	var out []Scenario
	for _, item := range splitList(raw) {
		s := Scenario(item)
		if !valid[s] {
			return nil, fmt.Errorf("VENDOR_TESTS_SCENARIOS: unknown scenario %q", item)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseVendorFilter(raw string) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	known := map[string]bool{}
	for _, l := range vendorLoaders() {
		known[l.key] = true
	}
	out := map[string]bool{}
	for _, item := range splitList(raw) {
		if !known[item] && !strings.HasPrefix(item, "compat_") {
			return nil, fmt.Errorf("VENDOR_TESTS_VENDORS: unknown vendor %q", item)
		}
		out[item] = true
	}
	return out, nil
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinSurfaces(ss []Surface) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}

func env(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func envOr(k, def string) string {
	if v := env(k); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	v := env(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(k string, def int) int {
	v := env(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envDuration(k string, def time.Duration) (time.Duration, error) {
	v := env(k)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a duration (e.g. 120s, 2m): %w", k, v, err)
	}
	return d, nil
}

func nonEmpty(m map[ModelSlot]string) map[ModelSlot]string {
	out := map[ModelSlot]string{}
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func nonEmptyStr(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func must(s string, _ error) string { return s }
