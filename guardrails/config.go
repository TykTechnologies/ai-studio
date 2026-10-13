package guardrails

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
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
	// ActionQuarantine replaces the whole text of each flagged segment with
	// a placeholder and lets the request continue. Blocking on content a
	// client resends in its history (a tool result) bricks an agent loop:
	// every later request carries the same text.
	ActionQuarantine = "quarantine"
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
	DefaultTimeoutMs   = 2000
	DefaultPlaceholder = "[REDACTED:{{type}}]"
	// DefaultQuarantinePlaceholder; see Quarantine.Placeholder.
	DefaultQuarantinePlaceholder = "[content removed by policy: {{detector}} detected in {{source}}]"
	// MaxQuarantinePlaceholder bounds the placeholder template in characters.
	MaxQuarantinePlaceholder   = 1000
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

// Quarantine configures ActionQuarantine.
type Quarantine struct {
	// Placeholder is the template that replaces a quarantined segment.
	// {{detector}} expands to the detectors that flagged it, {{category}} to
	// their categories, {{source}} to where the text came from ("tool result
	// from read_file", "user message") and {{tool_name}} to the tool's name.
	// Values are reduced to letters, digits and _ . : - before they are
	// inserted, so a tool name chosen by a client cannot write instructions
	// into the placeholder. Nil selects DefaultQuarantinePlaceholder; an
	// empty template is rejected.
	Placeholder *string `json:"placeholder,omitempty"`
}

// ToolResults configures how a request guardrail treats tool results: the
// output of tools the client ran (OpenAI role tool messages, Anthropic
// tool_result blocks, Google functionResponse parts), the main channel for
// indirect prompt injection.
type ToolResults struct {
	// Scan sends tool results to the provider whatever the scope, across
	// the whole history: the client resends a tool result on every turn, so
	// scanning only the current turn would protect one turn. Nil means true,
	// so stored configs from before tool results were scanned do scan them.
	// False keeps the selection by scope that applied before: tool results
	// only under all_messages.
	Scan *bool `json:"scan,omitempty"`
	// OnDetect is the action for a flagged tool result. Empty means the
	// filter's on_detect, except that block becomes quarantine: blocking a
	// request for a tool result its client resends every turn stops the
	// agent for good.
	OnDetect string `json:"on_detect,omitempty"`
}

// ScansToolResults reports whether tool results are scanned in the whole
// history under every scope.
func (c Config) ScansToolResults() bool {
	return c.ToolResults.Scan == nil || *c.ToolResults.Scan
}

// ActionFor is the action for a flagged segment of the given kind.
func (c Config) ActionFor(kind string) string {
	if kind != KindToolResult {
		return c.OnDetect
	}
	if c.ToolResults.OnDetect != "" {
		return c.ToolResults.OnDetect
	}
	if c.OnDetect == ActionBlock {
		return ActionQuarantine
	}
	return c.OnDetect
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
	Exclude      []string    `json:"exclude,omitempty"`
	Scope        string      `json:"scope,omitempty"`
	OnDetect     string      `json:"on_detect"`
	Redaction    Redaction   `json:"redaction,omitempty"`
	Quarantine   Quarantine  `json:"quarantine,omitempty"`
	ToolResults  ToolResults `json:"tool_results,omitempty"`
	FailMode     string      `json:"fail_mode,omitempty"`
	TimeoutMs    int         `json:"timeout_ms,omitempty"`
	Stream       Stream      `json:"stream,omitempty"`
	BlockMessage string      `json:"block_message,omitempty"`
	Budget       Budget      `json:"budget,omitzero"`
	// Cache, when false, keeps the filter's verdicts out of the verdict
	// cache: for a provider whose verdict depends on more than the text (an
	// HTTP classifier that reads the request metadata). Nil means true.
	Cache *bool `json:"cache,omitempty"`
	// OnDetectExt is the edge snapshot's carrier for an action edges before
	// v2.3 do not know (quarantine): those edges read on_detect, which then
	// holds block, so they block on a hit rather than reject the config and
	// fail open or closed. ParseConfig folds it back into OnDetect; it is
	// never stored.
	OnDetectExt string `json:"on_detect_ext,omitempty"`
}

// Budget bounds what a request-side guardrail spends on one request with a
// remote provider. Text beyond a budget is never let through as clean: a
// segment over max_scan_chars or beyond max_pieces, or left unscanned when
// time_ms runs out in a run of several provider calls, is flagged as
// guardrail.unscanned and gets the filter's action, whatever the fail mode.
// Local providers scan everything (up to 16 MiB per segment) and ignore it.
type Budget struct {
	// MaxScanChars is the most characters of one segment sent for
	// scanning. Default DefaultBudgetMaxScanChars.
	MaxScanChars int `json:"max_scan_chars,omitempty"`
	// MaxPieces is the most pieces (segments, or parts of segments longer
	// than the provider takes in one go) one request sends to the provider.
	// Verdicts from the cache do not count. Default DefaultBudgetMaxPieces.
	MaxPieces int `json:"max_pieces,omitempty"`
	// TimeMs is the wall time for all of one request's provider calls, which
	// run several at a time. At least timeout_ms, which bounds each call;
	// default BudgetTimeoutMultiple times timeout_ms.
	TimeMs int `json:"time_ms,omitempty"`
	// SingleCallChars is the largest request, in characters sent for
	// scanning, whose provider failure (timeout, 5xx, 429, network) the fail
	// mode decides, provided it fits one provider call. A larger request's
	// failed text is unscanned instead: whoever sends it can make it slow.
	// Default DefaultBudgetSingleCallChars, at most max_scan_chars.
	SingleCallChars int `json:"single_call_chars,omitempty"`
}

// Budget defaults and bounds.
const (
	DefaultBudgetMaxScanChars = 256 << 10
	MaxBudgetScanChars        = 16 << 20
	DefaultBudgetMaxPieces    = 256
	// DefaultBudgetSingleCallChars is about an ordinary prompt.
	DefaultBudgetSingleCallChars = 32 << 10
	MaxBudgetPieces              = 10000
	BudgetTimeoutMultiple        = 4
	MaxBudgetTimeMs              = 300_000
)

// ParseConfig decodes a stored config map. It does not validate; call
// Normalize for that.
func ParseConfig(raw map[string]any) (Config, error) {
	var cfg Config
	if raw == nil {
		return cfg, fmt.Errorf("%w is required", ErrInvalidConfig)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	// The edge snapshot carries actions older edges do not know in
	// on_detect_ext (see ConfigJSONForEdge). One this build does not know
	// either, from a newer hub, leaves on_detect, the hub's safe fallback.
	if knownActions[cfg.OnDetectExt] {
		cfg.OnDetect = cfg.OnDetectExt
	}
	cfg.OnDetectExt = ""
	return cfg, nil
}

// knownActions are the on_detect values this build applies.
var knownActions = map[string]bool{ActionBlock: true, ActionRedact: true, ActionLog: true, ActionQuarantine: true}

// legacyActions are the on_detect values every edge release understands.
// An edge rejects a config with any other value, so the snapshot never
// sends one there.
var legacyActions = map[string]bool{ActionBlock: true, ActionRedact: true, ActionLog: true}

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
		return cfg, fmt.Errorf("%w: at least one detector is required", ErrInvalidConfig)
	}
	known := map[string]bool{}
	for _, d := range spec.Detectors {
		known[d.Name] = true
	}
	for i, d := range cfg.Detectors {
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" {
			return cfg, fmt.Errorf("%w: detectors[%d] has no name", ErrInvalidConfig, i)
		}
		if !spec.OpenDetectors && !known[d.Name] {
			return cfg, fmt.Errorf("%w: detector %q is not offered by provider %q", ErrInvalidConfig, d.Name, cfg.Provider)
		}
		cfg.Detectors[i] = d
	}

	anyConnection := false
	for _, f := range spec.ConnectionFields {
		v := strings.TrimSpace(cfg.Connection[f.Name])
		if f.Required && v == "" {
			return cfg, fmt.Errorf("%w: connection.%s is required for provider %q", ErrInvalidConfig, f.Name, cfg.Provider)
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
		return cfg, fmt.Errorf("%w: provider %q needs at least one of connection.%s", ErrInvalidConfig, cfg.Provider, strings.Join(names, ", connection."))
	}

	switch cfg.OnDetect {
	case ActionBlock, ActionLog:
	case ActionRedact:
		if !spec.Redacts {
			return cfg, fmt.Errorf("%w: provider %q cannot redact; use block or log", ErrInvalidConfig, cfg.Provider)
		}
	case ActionQuarantine:
		// A response cannot continue without the text it was flagged for;
		// tool output filters are response filters too.
		if responseFilter {
			return cfg, fmt.Errorf("%w: quarantine applies to requests only; use block or log on responses", ErrInvalidConfig)
		}
	case "":
		return cfg, fmt.Errorf("%w: on_detect is required (block, redact, quarantine or log)", ErrInvalidConfig)
	default:
		return cfg, fmt.Errorf("%w: on_detect %q is not one of block, redact, quarantine, log", ErrInvalidConfig, cfg.OnDetect)
	}

	switch cfg.ToolResults.OnDetect {
	case "", ActionBlock, ActionLog:
	case ActionRedact:
		if !spec.Redacts {
			return cfg, fmt.Errorf("%w: provider %q cannot redact; use block, quarantine or log for tool_results.on_detect", ErrInvalidConfig, cfg.Provider)
		}
	case ActionQuarantine:
		if responseFilter {
			return cfg, fmt.Errorf("%w: quarantine applies to requests only; use block or log on responses", ErrInvalidConfig)
		}
	default:
		return cfg, fmt.Errorf("%w: tool_results.on_detect %q is not one of block, redact, quarantine, log", ErrInvalidConfig, cfg.ToolResults.OnDetect)
	}

	if p := cfg.Quarantine.Placeholder; p == nil {
		def := DefaultQuarantinePlaceholder
		cfg.Quarantine.Placeholder = &def
	} else if strings.TrimSpace(*p) == "" {
		return cfg, fmt.Errorf("%w: quarantine.placeholder must not be empty", ErrInvalidConfig)
	} else if utf8.RuneCountInString(*p) > MaxQuarantinePlaceholder {
		return cfg, fmt.Errorf("%w: quarantine.placeholder is longer than %d characters", ErrInvalidConfig, MaxQuarantinePlaceholder)
	}

	switch cfg.Scope {
	case "":
		cfg.Scope = ScopeAllUser
	case ScopeLastUser, ScopeAllUser, ScopeSystemAndUser, ScopeAllMessages:
	default:
		return cfg, fmt.Errorf("%w: scope %q is not one of last_user, all_user, system_and_user, all_messages", ErrInvalidConfig, cfg.Scope)
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
		return cfg, fmt.Errorf("%w: fail_mode %q is not one of open, closed", ErrInvalidConfig, cfg.FailMode)
	}

	switch cfg.Redaction.Style {
	case "":
		cfg.Redaction.Style = RedactionPlaceholder
	case RedactionPlaceholder, RedactionMask, RedactionHash:
	default:
		return cfg, fmt.Errorf("%w: redaction.style %q is not one of placeholder, mask, hash", ErrInvalidConfig, cfg.Redaction.Style)
	}
	if cfg.Redaction.Placeholder == "" {
		cfg.Redaction.Placeholder = DefaultPlaceholder
	}

	if cfg.TimeoutMs < 0 {
		return cfg, fmt.Errorf("%w: timeout_ms must not be negative", ErrInvalidConfig)
	}
	if cfg.TimeoutMs == 0 {
		cfg.TimeoutMs = DefaultTimeoutMs
	}
	if cfg.Stream.EvaluateEveryChars < 0 {
		return cfg, fmt.Errorf("%w: stream.evaluate_every_chars must not be negative", ErrInvalidConfig)
	}
	if cfg.BlockMessage == "" {
		if responseFilter {
			cfg.BlockMessage = DefaultResponseBlockMsg
		} else {
			cfg.BlockMessage = DefaultRequestBlockMessage
		}
	}
	if err := cfg.Budget.normalize(cfg.TimeoutMs); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// normalize validates the budget and fills its defaults; timeoutMs is the
// filter's normalised per-call timeout.
func (b *Budget) normalize(timeoutMs int) error {
	if b.MaxScanChars < 0 || b.MaxScanChars > MaxBudgetScanChars {
		return fmt.Errorf("%w: budget.max_scan_chars must be between 1 and %d", ErrInvalidConfig, MaxBudgetScanChars)
	}
	if b.MaxScanChars == 0 {
		b.MaxScanChars = DefaultBudgetMaxScanChars
	}
	if b.MaxPieces < 0 || b.MaxPieces > MaxBudgetPieces {
		return fmt.Errorf("%w: budget.max_pieces must be between 1 and %d", ErrInvalidConfig, MaxBudgetPieces)
	}
	if b.MaxPieces == 0 {
		b.MaxPieces = DefaultBudgetMaxPieces
	}
	maxTime := max(MaxBudgetTimeMs, timeoutMs)
	if b.TimeMs == 0 {
		b.TimeMs = max(timeoutMs, min(BudgetTimeoutMultiple*timeoutMs, MaxBudgetTimeMs))
	}
	if b.TimeMs < timeoutMs || b.TimeMs > maxTime {
		return fmt.Errorf("%w: budget.time_ms must be between timeout_ms (%d) and %d", ErrInvalidConfig, timeoutMs, maxTime)
	}
	if b.SingleCallChars < 0 || b.SingleCallChars > b.MaxScanChars {
		return fmt.Errorf("%w: budget.single_call_chars must be between 1 and max_scan_chars (%d)", ErrInvalidConfig, b.MaxScanChars)
	}
	if b.SingleCallChars == 0 {
		b.SingleCallChars = min(DefaultBudgetSingleCallChars, b.MaxScanChars)
	}
	return nil
}

// CacheEnabled reports whether the filter's verdicts may be cached.
func (c Config) CacheEnabled() bool {
	return c.Cache == nil || *c.Cache
}

// ForStorage is what is stored for a config the admin wrote (parsed) once
// Normalize has validated it (normalized): the normalised config, except
// that the budget keeps only the values the admin set. Budget defaults are
// derived from other settings (time_ms from timeout_ms, single_call_chars
// from max_scan_chars), so storing them would pin values a later edit of
// those settings then contradicts; they are derived where the filter runs.
func ForStorage(parsed, normalized Config) Config {
	normalized.Budget = parsed.Budget
	return normalized
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
