package guardrails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// DetectorSpec describes one detector a provider offers, for the API and the
// admin UI.
type DetectorSpec struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// HasThreshold reports whether DetectorConfig.Threshold applies, and
	// ThresholdHint what it means ("0..1 score", "0..6 severity").
	HasThreshold  bool   `json:"has_threshold"`
	ThresholdHint string `json:"threshold_hint,omitempty"`
}

// ConnectionField describes one key of Config.Connection.
type ConnectionField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	// Secret marks a field that holds a credential. The UI offers the secret
	// picker and the value is expected to be a $SECRET/ or $ENV/ reference.
	Secret bool `json:"secret"`
	// Example is placeholder text for the UI.
	Example string `json:"example,omitempty"`
}

// Spec describes a provider for the API and the admin UI.
type Spec struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// Redacts reports whether ActionRedact is available.
	Redacts bool `json:"redacts"`
	// Local reports an in-process provider (no network call).
	Local bool `json:"local"`
	// Detectors is the fixed catalogue. When OpenDetectors is true the
	// provider also accepts names outside it (the generic HTTP provider,
	// whose server defines its own).
	Detectors        []DetectorSpec    `json:"detectors"`
	OpenDetectors    bool              `json:"open_detectors"`
	ConnectionFields []ConnectionField `json:"connection_fields"`
	// RequireAnyConnection demands at least one connection field even though
	// none is individually required (Lakera: an API key for the SaaS or an
	// endpoint for a self-hosted Guard).
	RequireAnyConnection bool `json:"require_any_connection"`
	// DocsURL points at the provider's page in the documentation.
	DocsURL string `json:"docs_url,omitempty"`
	// Available reports whether an implementation is registered in this
	// build. False in the Community Edition.
	Available bool `json:"available"`
}

// Factory builds a provider from a normalised config.
type Factory func(cfg Config) (Provider, error)

var (
	registryMu sync.RWMutex
	factories  = map[string]Factory{}
	specs      = map[string]Spec{}
)

// RegisterSpec publishes a provider's description. Specs are registered by
// this package for every provider it knows about, in every build, so the
// API can describe them even where they are not implemented.
func RegisterSpec(s Spec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	specs[s.Name] = s
}

// RegisterFactory installs the implementation of a provider. Called from the
// init() of each provider package; the Enterprise build imports them.
func RegisterFactory(name string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[name] = f
	providerCache = sync.Map{}
}

// ProviderSpec returns the description of a provider.
func ProviderSpec(name string) (Spec, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := specs[name]
	if !ok {
		return Spec{}, false
	}
	_, s.Available = factories[name]
	return s, true
}

// Specs lists every known provider, sorted by name, with Available set for
// the ones this build implements.
func Specs() []Spec {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Spec, 0, len(specs))
	for name, s := range specs {
		_, s.Available = factories[name]
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Available reports whether any provider implementation is registered, i.e.
// whether this build enforces guardrail filters at all.
func Available() bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return len(factories) > 0
}

// providerCache holds one provider instance per distinct config, so a remote
// provider keeps its HTTP client and the built-in one its compiled pattern
// set across requests.
var providerCache sync.Map // config key -> Provider

func configKey(cfg Config) string {
	b, _ := json.Marshal(cfg)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// NewProvider returns the provider for a normalised config, building and
// caching it on first use.
func NewProvider(cfg Config) (Provider, error) {
	key := configKey(cfg)
	if p, ok := providerCache.Load(key); ok {
		return p.(Provider), nil
	}

	registryMu.RLock()
	f, ok := factories[cfg.Provider]
	none := len(factories) == 0
	registryMu.RUnlock()
	if none {
		return nil, ErrNoProviders
	}
	if !ok {
		return nil, ErrUnknownProvider
	}

	p, err := f(cfg)
	if err != nil {
		return nil, err
	}
	actual, _ := providerCache.LoadOrStore(key, p)
	return actual.(Provider), nil
}

// Classify runs the provider for cfg on in, bounded by cfg.TimeoutMs, and
// stamps the verdict's latency. Errors from the provider, including a
// timeout, are returned to the caller, which applies the filter's fail mode.
func Classify(ctx context.Context, cfg Config, p Provider, in Input) (Verdict, error) {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := time.Now()
	v, err := p.Classify(ctx, in)
	v.Latency = time.Since(start)
	if err != nil {
		return v, err
	}
	if !v.Flagged && len(v.Findings) > 0 {
		v.Flagged = true
	}
	return v, nil
}
