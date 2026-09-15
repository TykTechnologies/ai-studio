package guardrails

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Provider names. The catalogue in providers.go describes each one for the
// API and the admin UI; the Enterprise build registers the implementations.
const (
	ProviderBuiltin            = "builtin"
	ProviderHTTP               = "http"
	ProviderPresidio           = "presidio"
	ProviderLakera             = "lakera"
	ProviderAzureContentSafety = "azure_content_safety"
	ProviderAzurePII           = "azure_pii"
	ProviderBedrockGuardrails  = "bedrock_guardrails"
)

// Actions taken when a provider flags the text.
const (
	ActionBlock  = "block"
	ActionRedact = "redact"
	ActionLog    = "log"
)

// Scopes select which messages of a request are sent to the provider. On the
// response side and on tool calls there is only one text, and the scope is
// ignored.
const (
	ScopeLastUser      = "last_user"
	ScopeAllUser       = "all_user"
	ScopeSystemAndUser = "system_and_user"
	ScopeAllMessages   = "all_messages"
)

// Fail modes decide what happens when the provider errors or times out.
const (
	FailOpen   = "open"
	FailClosed = "closed"
)

// Redaction styles.
const (
	RedactionPlaceholder = "placeholder"
	RedactionMask        = "mask"
	RedactionHash        = "hash"
)

const (
	DefaultTimeoutMs           = 2000
	DefaultPlaceholder         = "[REDACTED:{{type}}]"
	DefaultRequestBlockMessage = "Request blocked by policy"
	DefaultResponseBlockMsg    = "Response blocked by policy"
	// DefaultStreamEvery is the streaming cadence for remote providers;
	// DefaultStreamEveryLocal for in-process ones.
	DefaultStreamEvery      = 1000
	DefaultStreamEveryLocal = 250
)

// DetectorConfig enables one detector (or a whole category for the built-in
// provider) with an optional provider-specific threshold.
type DetectorConfig struct {
	Name string `json:"name"`
	// Threshold is provider-specific: a 0..1 score for classifiers, a 0..6
	// severity for Azure Content Safety. Nil means the provider's default.
	Threshold *float64 `json:"threshold,omitempty"`
}

// Redaction configures the rewrite applied on ActionRedact.
type Redaction struct {
	Style       string `json:"style,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Stream configures evaluation cadence on streaming responses.
type Stream struct {
	// EvaluateEveryChars re-runs the provider each time the accumulated
	// response grows by this many characters. 0 selects the provider default.
	EvaluateEveryChars int `json:"evaluate_every_chars,omitempty"`
}

// Config is the stored configuration of a guardrail filter
// (models.Filter.Config when Kind is "guardrail").
type Config struct {
	Provider   string            `json:"provider"`
	Connection map[string]string `json:"connection,omitempty"`
	Detectors  []DetectorConfig  `json:"detectors"`
	// Exclude names detectors to leave out when a category is enabled, e.g.
	// "pii.ipv4" while "pii" is on. Built-in provider only.
	Exclude      []string  `json:"exclude,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	OnDetect     string    `json:"on_detect"`
	Redaction    Redaction `json:"redaction,omitempty"`
	FailMode     string    `json:"fail_mode,omitempty"`
	TimeoutMs    int       `json:"timeout_ms,omitempty"`
	Stream       Stream    `json:"stream,omitempty"`
	BlockMessage string    `json:"block_message,omitempty"`
}

// ParseConfig decodes a stored config map. It does not validate; call
// Normalize for that.
func ParseConfig(raw map[string]any) (Config, error) {
	var cfg Config
	if raw == nil {
		return cfg, fmt.Errorf("guardrail config is required")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("guardrail config: %w", err)
	}
	return cfg, nil
}

// Normalize validates cfg against the provider catalogue and fills defaults
// that depend on the filter's direction: fail mode is closed on the request
// side and open on the response side, matching how script filters already
// behave, and the block message names the side.
func Normalize(cfg Config, responseFilter bool) (Config, error) {
	spec, ok := ProviderSpec(cfg.Provider)
	if !ok {
		return cfg, fmt.Errorf("%w: %q", ErrUnknownProvider, cfg.Provider)
	}

	if len(cfg.Detectors) == 0 {
		return cfg, fmt.Errorf("guardrail config: at least one detector is required")
	}
	known := map[string]bool{}
	for _, d := range spec.Detectors {
		known[d.Name] = true
	}
	for i, d := range cfg.Detectors {
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" {
			return cfg, fmt.Errorf("guardrail config: detectors[%d] has no name", i)
		}
		if !spec.OpenDetectors && !known[d.Name] {
			return cfg, fmt.Errorf("guardrail config: detector %q is not offered by provider %q", d.Name, cfg.Provider)
		}
		cfg.Detectors[i] = d
	}

	anyConnection := false
	for _, f := range spec.ConnectionFields {
		v := strings.TrimSpace(cfg.Connection[f.Name])
		if f.Required && v == "" {
			return cfg, fmt.Errorf("guardrail config: connection.%s is required for provider %q", f.Name, cfg.Provider)
		}
		if v != "" {
			anyConnection = true
		}
	}
	if spec.RequireAnyConnection && !anyConnection {
		names := make([]string, 0, len(spec.ConnectionFields))
		for _, f := range spec.ConnectionFields {
			names = append(names, f.Name)
		}
		return cfg, fmt.Errorf("guardrail config: provider %q needs at least one of connection.%s", cfg.Provider, strings.Join(names, ", connection."))
	}

	switch cfg.OnDetect {
	case ActionBlock, ActionLog:
	case ActionRedact:
		if !spec.Redacts {
			return cfg, fmt.Errorf("guardrail config: provider %q cannot redact; use block or log", cfg.Provider)
		}
	case "":
		return cfg, fmt.Errorf("guardrail config: on_detect is required (block, redact or log)")
	default:
		return cfg, fmt.Errorf("guardrail config: on_detect %q is not one of block, redact, log", cfg.OnDetect)
	}

	switch cfg.Scope {
	case "":
		cfg.Scope = ScopeAllUser
	case ScopeLastUser, ScopeAllUser, ScopeSystemAndUser, ScopeAllMessages:
	default:
		return cfg, fmt.Errorf("guardrail config: scope %q is not one of last_user, all_user, system_and_user, all_messages", cfg.Scope)
	}

	switch cfg.FailMode {
	case "":
		if responseFilter {
			cfg.FailMode = FailOpen
		} else {
			cfg.FailMode = FailClosed
		}
	case FailOpen, FailClosed:
	default:
		return cfg, fmt.Errorf("guardrail config: fail_mode %q is not one of open, closed", cfg.FailMode)
	}

	switch cfg.Redaction.Style {
	case "":
		cfg.Redaction.Style = RedactionPlaceholder
	case RedactionPlaceholder, RedactionMask, RedactionHash:
	default:
		return cfg, fmt.Errorf("guardrail config: redaction.style %q is not one of placeholder, mask, hash", cfg.Redaction.Style)
	}
	if cfg.Redaction.Placeholder == "" {
		cfg.Redaction.Placeholder = DefaultPlaceholder
	}

	if cfg.TimeoutMs < 0 {
		return cfg, fmt.Errorf("guardrail config: timeout_ms must not be negative")
	}
	if cfg.TimeoutMs == 0 {
		cfg.TimeoutMs = DefaultTimeoutMs
	}
	if cfg.Stream.EvaluateEveryChars < 0 {
		return cfg, fmt.Errorf("guardrail config: stream.evaluate_every_chars must not be negative")
	}
	if cfg.BlockMessage == "" {
		if responseFilter {
			cfg.BlockMessage = DefaultResponseBlockMsg
		} else {
			cfg.BlockMessage = DefaultRequestBlockMessage
		}
	}
	return cfg, nil
}

// ToMap renders cfg the way it is stored on the filter.
func (c Config) ToMap() (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// DetectorNames returns the enabled detector names in a stable order.
func (c Config) DetectorNames() []string {
	names := make([]string, 0, len(c.Detectors))
	for _, d := range c.Detectors {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return names
}

// Threshold returns the configured threshold for a detector, or def.
func (c Config) Threshold(detector string, def float64) float64 {
	for _, d := range c.Detectors {
		if d.Name == detector && d.Threshold != nil {
			return *d.Threshold
		}
	}
	return def
}

// EvaluateEvery returns the streaming cadence in characters for a provider
// with the given capabilities.
func (c Config) EvaluateEvery(caps Capabilities) int {
	if c.Stream.EvaluateEveryChars > 0 {
		return c.Stream.EvaluateEveryChars
	}
	if caps.Local {
		return DefaultStreamEveryLocal
	}
	return DefaultStreamEvery
}
