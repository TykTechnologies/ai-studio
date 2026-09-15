package guardrails

import (
	"container/list"
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
	providerCache.reset()
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
// set across requests. It is bounded: every edit of a filter's config is a
// new key, and the entries for configurations nobody runs any more are the
// least recently used, so they are the ones dropped.
const providerCacheSize = 256

var providerCache = newProviderLRU(providerCacheSize)

type providerLRU struct {
	mu    sync.Mutex
	max   int
	order *list.List // front is most recently used
	items map[string]*list.Element
}

type providerEntry struct {
	key      string
	provider Provider
}

func newProviderLRU(max int) *providerLRU {
	return &providerLRU{max: max, order: list.New(), items: make(map[string]*list.Element)}
}

// get returns the cached provider for key and marks it recently used.
func (c *providerLRU) get(key string) (Provider, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*providerEntry).provider, true
}

// add stores p under key unless another goroutine got there first, in which
// case the existing provider wins so every caller shares one instance. The
// least recently used entry is dropped once the cache is over capacity.
func (c *providerLRU) add(key string, p Provider) Provider {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*providerEntry).provider
	}
	c.items[key] = c.order.PushFront(&providerEntry{key: key, provider: p})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*providerEntry).key)
	}
	return p
}

func (c *providerLRU) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// reset drops every entry; a newly registered factory must not be shadowed
// by providers built before it existed.
func (c *providerLRU) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = make(map[string]*list.Element)
}

func configKey(cfg Config) string {
	b, _ := json.Marshal(cfg)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// NewProvider returns the provider for a normalised config, building and
// caching it on first use.
func NewProvider(cfg Config) (Provider, error) {
	key := configKey(cfg)
	if p, ok := providerCache.get(key); ok {
		return p, nil
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
	return providerCache.add(key, p), nil
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
