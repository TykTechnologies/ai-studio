package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AgentTracingConfig is the global Agent Call Tracing configuration
// (Enterprise feature; spec enterprise/features/agenttracing/SPEC.md §11).
// Every key is an AGENT_TRACING_* environment variable. Per-App and
// per-resource overrides live in the database, not here.
//
// The feature runs only when the Enterprise implementation is linked in,
// the licence is valid and Enabled is true (services/agenttracing).
type AgentTracingConfig struct {
	// Enabled is AGENT_TRACING_ENABLED (default false).
	Enabled bool
	// Key is AGENT_TRACING_KEY, the root key the HMAC and payload keys are
	// derived from with HKDF (D4). Empty falls back to TYK_AI_SECRET_KEY;
	// tracing refuses to start with neither. Never logged.
	Key string `json:"-"`
	// PreviousKey is AGENT_TRACING_KEY_PREVIOUS: the root key before a
	// rotation. Payloads encrypted under it stay readable, and correlation
	// lookups try it too. Never logged.
	PreviousKey string `json:"-"`
	// FallbackKey is the secrets key (TYK_AI_SECRET_KEY, or the embedding
	// host's SecretKey), used when Key is empty. The host sets it; it is not
	// read from the environment here.
	FallbackKey string `json:"-"`

	// Capture queue and stream caps (§10).
	QueueMaxEvents       int
	QueueMaxBytes        int64
	MaxStreamBufferBytes int

	// Content capture (§5.5). Modes: off < metadata < hashed < redacted < full.
	// ContentCeiling is the highest mode any App, request header or the
	// default may reach; ContentDefaultMode applies to Apps without their
	// own mode and never exceeds the ceiling (jyx.172). An unset ceiling is
	// the higher of the default mode and the request override
	// (EffectiveContentCeiling).
	ContentCeiling            string
	ContentDefaultMode        string
	ContentMaxRequestOverride string
	// StoreContentModeCap is the highest mode ever persisted, whatever the
	// App allows.
	StoreContentModeCap string
	RedactionDetectors  []string

	// Payload storage (§3.4).
	PayloadBackend      string // db (Postgres or SQLite) | filesystem
	PayloadMaxPartBytes int
	PayloadMaxSpanBytes int
	PayloadStoreBinary  bool

	// Correlation (§4).
	MinLinkConfidence      string // declared | exact | strong | weak
	ToolMatchWindow        time.Duration
	RetryWindow            time.Duration
	ClockSkewTolerance     time.Duration
	TurnIdleTimeout        time.Duration
	SessionIdleTimeout     time.Duration
	CompatHeaders          []string
	HarnessSessionHeaders  []string
	DelegationToolPatterns []string
	ToolNamePrefixStrip    []string

	// Outbound propagation (§4.6, amended by Q11: LLM providers default on,
	// with a per-LLM opt-out).
	PropagateMCP           bool
	PropagateHTTPUpstreams bool
	PropagateLLMProviders  bool
	OpenAIClientRequestID  bool
	ForwardTykHeaders      bool
	BaggageAllowlist       []string

	// Retention (§3.4; D5 Q2a). Payloads never outlive metadata.
	MetadataRetentionDays int
	PayloadRetentionDays  int
	CorrelationKeyTTL     time.Duration

	// StoreSampleRate is the fraction of sessions persisted (0..1).
	StoreSampleRate float64

	// OTel export (§5). Exporters are listed in a YAML file
	// (ExportersFile, AGENT_TRACING_EXPORTERS_FILE) using the §11
	// "exporters:" schema; the OTel export bead parses it.
	ExportSemconvProfile             string
	ExportAgentSpanMode              string // tyk | semconv
	ExportGatewayEvents              string // events | spans
	ExportContent                    string // none | reference | inline
	ExportMaxAttributeBytes          int
	ExportMaxSpanContentBytes        int
	ExportMaxDelay                   time.Duration
	ExportInferredSessionAsSessionID bool
	ExportConversationIDFallback     string // never | inferred
	ExportUserIDMode                 string // user.id to exporters: off | hashed (keyed pseudonym, default) | raw (jyx.173)
	ExportersFile                    string

	// Agent metrics (§5.7).
	MetricsEnabled        bool
	MetricsIncludeAppID   bool
	MetricsExportInterval time.Duration

	// UIMaxPayloadViewBytes caps a payload rendered inline in the UI.
	UIMaxPayloadViewBytes int
}

// Agent tracing content modes, in disclosure order (§5.5).
var agentTracingContentModes = []string{"off", "metadata", "hashed", "redacted", "full"}

// AgentTracingContentModeRank returns a mode's place in the disclosure order
// (off = 0), or -1 for an unknown mode.
func AgentTracingContentModeRank(mode string) int {
	for i, m := range agentTracingContentModes {
		if m == mode {
			return i
		}
	}
	return -1
}

// EffectiveContentCeiling is the content ceiling in force: the configured
// one or, when none is set, the higher of the default mode and the request
// override, so a header may still raise capture as far as before the split
// (jyx.172).
func (c AgentTracingConfig) EffectiveContentCeiling() string {
	if c.ContentCeiling != "" {
		return c.ContentCeiling
	}
	if AgentTracingContentModeRank(c.ContentMaxRequestOverride) > AgentTracingContentModeRank(c.ContentDefaultMode) {
		return c.ContentMaxRequestOverride
	}
	return c.ContentDefaultMode
}

// DefaultAgentTracingConfig returns the §11 defaults (as amended by D5 and Q11).
func DefaultAgentTracingConfig() AgentTracingConfig {
	return AgentTracingConfig{
		QueueMaxEvents:       10000,
		QueueMaxBytes:        64 << 20,
		MaxStreamBufferBytes: 1 << 20,

		ContentDefaultMode:        "off",
		ContentMaxRequestOverride: "metadata",
		StoreContentModeCap:       "full",
		RedactionDetectors: []string{"email", "phone", "payment_card", "iban", "us_ssn",
			"jwt", "bearer_token", "api_key", "private_key"},

		PayloadBackend:      "db",
		PayloadMaxPartBytes: 256 << 10,
		PayloadMaxSpanBytes: 1 << 20,

		MinLinkConfidence:  "weak",
		ToolMatchWindow:    300 * time.Second,
		RetryWindow:        30 * time.Second,
		ClockSkewTolerance: 2 * time.Second,
		TurnIdleTimeout:    5 * time.Minute,
		SessionIdleTimeout: 30 * time.Minute,
		CompatHeaders: []string{"helicone", "portkey", "openrouter", "bifrost", "agent_router",
			"cloudflare", "agentcore", "mcp_session"},
		// The coding-harness extractors verified against recorded traffic
		// (bead jyx.52, testdata/agent_tracing/harness); remove one to switch
		// it off. Other entries are header names read as a session id.
		HarnessSessionHeaders:  []string{"claude_code", "codex_cli", "opencode"},
		DelegationToolPatterns: []string{`^(task|agent|delegate.*|transfer_to_.*|handoff.*|spawn.*|run_?subagent)$`},
		ToolNamePrefixStrip:    []string{"mcp__{server}__", "{server}__", "{server}."},

		PropagateMCP:           true,
		PropagateHTTPUpstreams: true,
		PropagateLLMProviders:  true,
		OpenAIClientRequestID:  true,
		BaggageAllowlist:       []string{"session.id"},

		MetadataRetentionDays: 30,
		PayloadRetentionDays:  7,
		CorrelationKeyTTL:     24 * time.Hour,

		StoreSampleRate: 1.0,

		ExportSemconvProfile:             "genai-1.41",
		ExportAgentSpanMode:              "tyk",
		ExportGatewayEvents:              "events",
		ExportContent:                    "none",
		ExportMaxAttributeBytes:          16384,
		ExportMaxSpanContentBytes:        131072,
		ExportMaxDelay:                   30 * time.Second,
		ExportInferredSessionAsSessionID: true,
		ExportConversationIDFallback:     "never",
		ExportUserIDMode:                 "hashed",

		MetricsEnabled:        true,
		MetricsIncludeAppID:   true,
		MetricsExportInterval: 30 * time.Second,

		UIMaxPayloadViewBytes: 2 << 20,
	}
}

func getAgentTracingConfig(getenv func(string) string) AgentTracingConfig {
	cfg, warnings := parseAgentTracingConfig(getenv)
	for _, w := range warnings {
		cfgLog.Warn().Msg(w)
	}
	return cfg
}

// parseAgentTracingConfig reads AGENT_TRACING_*. Invalid values fall back
// to the default and add a warning; they never fail start-up.
func parseAgentTracingConfig(getenv func(string) string) (AgentTracingConfig, []string) {
	cfg := DefaultAgentTracingConfig()
	p := atParser{getenv: getenv}

	p.boolean("AGENT_TRACING_ENABLED", &cfg.Enabled)
	cfg.Key = getenv("AGENT_TRACING_KEY")
	cfg.PreviousKey = getenv("AGENT_TRACING_KEY_PREVIOUS")

	p.positiveInt("AGENT_TRACING_QUEUE_MAX_EVENTS", &cfg.QueueMaxEvents)
	p.positiveInt64("AGENT_TRACING_QUEUE_MAX_BYTES", &cfg.QueueMaxBytes)
	p.positiveInt("AGENT_TRACING_MAX_STREAM_BUFFER_BYTES", &cfg.MaxStreamBufferBytes)

	p.enum("AGENT_TRACING_CONTENT_DEFAULT_MODE", &cfg.ContentDefaultMode, agentTracingContentModes...)
	p.enum("AGENT_TRACING_CONTENT_CEILING", &cfg.ContentCeiling, agentTracingContentModes...)
	if cfg.ContentCeiling != "" && AgentTracingContentModeRank(cfg.ContentDefaultMode) > AgentTracingContentModeRank(cfg.ContentCeiling) {
		p.warn("AGENT_TRACING_CONTENT_DEFAULT_MODE (%s) is above AGENT_TRACING_CONTENT_CEILING (%s); using the ceiling",
			cfg.ContentDefaultMode, cfg.ContentCeiling)
		cfg.ContentDefaultMode = cfg.ContentCeiling
	}
	p.enum("AGENT_TRACING_CONTENT_MAX_REQUEST_OVERRIDE", &cfg.ContentMaxRequestOverride, agentTracingContentModes...)
	p.enum("AGENT_TRACING_STORE_CONTENT_MODE_CAP", &cfg.StoreContentModeCap, agentTracingContentModes...)
	p.list("AGENT_TRACING_REDACTION_DETECTORS", &cfg.RedactionDetectors)

	p.enum("AGENT_TRACING_PAYLOAD_BACKEND", &cfg.PayloadBackend, "db", "postgres", "filesystem")
	if cfg.PayloadBackend == "postgres" {
		cfg.PayloadBackend = "db" // the spec's name; the table works on SQLite too (D1)
	}
	p.positiveInt("AGENT_TRACING_PAYLOAD_MAX_PART_BYTES", &cfg.PayloadMaxPartBytes)
	p.positiveInt("AGENT_TRACING_PAYLOAD_MAX_SPAN_BYTES", &cfg.PayloadMaxSpanBytes)
	p.boolean("AGENT_TRACING_PAYLOAD_STORE_BINARY", &cfg.PayloadStoreBinary)
	if cfg.PayloadMaxPartBytes > cfg.PayloadMaxSpanBytes {
		p.warn("AGENT_TRACING_PAYLOAD_MAX_PART_BYTES (%d) is above AGENT_TRACING_PAYLOAD_MAX_SPAN_BYTES (%d); using the span cap",
			cfg.PayloadMaxPartBytes, cfg.PayloadMaxSpanBytes)
		cfg.PayloadMaxPartBytes = cfg.PayloadMaxSpanBytes
	}

	p.enum("AGENT_TRACING_MIN_LINK_CONFIDENCE", &cfg.MinLinkConfidence, "declared", "exact", "strong", "weak")
	p.positiveDuration("AGENT_TRACING_TOOL_MATCH_WINDOW", &cfg.ToolMatchWindow)
	p.positiveDuration("AGENT_TRACING_RETRY_WINDOW", &cfg.RetryWindow)
	p.positiveDuration("AGENT_TRACING_CLOCK_SKEW_TOLERANCE", &cfg.ClockSkewTolerance)
	p.positiveDuration("AGENT_TRACING_TURN_IDLE_TIMEOUT", &cfg.TurnIdleTimeout)
	p.positiveDuration("AGENT_TRACING_SESSION_IDLE_TIMEOUT", &cfg.SessionIdleTimeout)
	p.list("AGENT_TRACING_COMPAT_HEADERS", &cfg.CompatHeaders)
	p.list("AGENT_TRACING_HARNESS_SESSION_HEADERS", &cfg.HarnessSessionHeaders)
	p.regexList("AGENT_TRACING_DELEGATION_TOOL_PATTERNS", &cfg.DelegationToolPatterns)
	p.list("AGENT_TRACING_TOOL_NAME_PREFIX_STRIP", &cfg.ToolNamePrefixStrip)

	p.boolean("AGENT_TRACING_PROPAGATE_MCP", &cfg.PropagateMCP)
	p.boolean("AGENT_TRACING_PROPAGATE_HTTP_UPSTREAMS", &cfg.PropagateHTTPUpstreams)
	p.boolean("AGENT_TRACING_PROPAGATE_LLM_PROVIDERS", &cfg.PropagateLLMProviders)
	p.boolean("AGENT_TRACING_OPENAI_CLIENT_REQUEST_ID", &cfg.OpenAIClientRequestID)
	p.boolean("AGENT_TRACING_FORWARD_TYK_HEADERS", &cfg.ForwardTykHeaders)
	p.list("AGENT_TRACING_BAGGAGE_ALLOWLIST", &cfg.BaggageAllowlist)

	p.positiveInt("AGENT_TRACING_METADATA_RETENTION_DAYS", &cfg.MetadataRetentionDays)
	p.positiveInt("AGENT_TRACING_PAYLOAD_RETENTION_DAYS", &cfg.PayloadRetentionDays)
	if cfg.PayloadRetentionDays > cfg.MetadataRetentionDays {
		p.warn("AGENT_TRACING_PAYLOAD_RETENTION_DAYS (%d) is above AGENT_TRACING_METADATA_RETENTION_DAYS (%d); payloads never outlive metadata, using %d",
			cfg.PayloadRetentionDays, cfg.MetadataRetentionDays, cfg.MetadataRetentionDays)
		cfg.PayloadRetentionDays = cfg.MetadataRetentionDays
	}
	p.positiveDuration("AGENT_TRACING_CORRELATION_KEY_TTL", &cfg.CorrelationKeyTTL)

	p.fraction("AGENT_TRACING_STORE_SAMPLE_RATE", &cfg.StoreSampleRate)

	if v := strings.TrimSpace(getenv("AGENT_TRACING_EXPORT_SEMCONV_PROFILE")); v != "" {
		cfg.ExportSemconvProfile = v // a profile name or a path to a custom mapping YAML
	}
	p.enum("AGENT_TRACING_EXPORT_AGENT_SPAN_MODE", &cfg.ExportAgentSpanMode, "tyk", "semconv")
	p.enum("AGENT_TRACING_EXPORT_GATEWAY_EVENTS", &cfg.ExportGatewayEvents, "events", "spans")
	p.enum("AGENT_TRACING_EXPORT_CONTENT", &cfg.ExportContent, "none", "reference", "inline")
	p.positiveInt("AGENT_TRACING_EXPORT_MAX_ATTRIBUTE_BYTES", &cfg.ExportMaxAttributeBytes)
	p.positiveInt("AGENT_TRACING_EXPORT_MAX_SPAN_CONTENT_BYTES", &cfg.ExportMaxSpanContentBytes)
	p.positiveDuration("AGENT_TRACING_EXPORT_MAX_DELAY", &cfg.ExportMaxDelay)
	p.boolean("AGENT_TRACING_EXPORT_INFERRED_SESSION_AS_SESSION_ID", &cfg.ExportInferredSessionAsSessionID)
	p.enum("AGENT_TRACING_EXPORT_CONVERSATION_ID_FALLBACK", &cfg.ExportConversationIDFallback, "never", "inferred")
	// The earlier boolean still applies when the mode is not set:
	// true keeps the raw value, false turns it off (jyx.173).
	// An unparsable legacy value keeps the default, hashed (jyx.178).
	if _, set := p.get("AGENT_TRACING_EXPORT_USER_ID_MODE"); !set {
		if v, had := p.get("AGENT_TRACING_EXPORT_INCLUDE_USER_ID"); had {
			switch b, err := strconv.ParseBool(v); {
			case err != nil:
				p.warn("Invalid AGENT_TRACING_EXPORT_INCLUDE_USER_ID value %q; using AGENT_TRACING_EXPORT_USER_ID_MODE=%s", v, cfg.ExportUserIDMode)
			case b:
				cfg.ExportUserIDMode = "raw"
			default:
				cfg.ExportUserIDMode = "off"
			}
		}
	}
	p.enum("AGENT_TRACING_EXPORT_USER_ID_MODE", &cfg.ExportUserIDMode, "off", "hashed", "raw")
	cfg.ExportersFile = strings.TrimSpace(getenv("AGENT_TRACING_EXPORTERS_FILE"))

	p.boolean("AGENT_TRACING_METRICS_ENABLED", &cfg.MetricsEnabled)
	p.boolean("AGENT_TRACING_METRICS_INCLUDE_APP_ID", &cfg.MetricsIncludeAppID)
	p.positiveDuration("AGENT_TRACING_METRICS_EXPORT_INTERVAL", &cfg.MetricsExportInterval)

	p.positiveInt("AGENT_TRACING_UI_MAX_PAYLOAD_VIEW_BYTES", &cfg.UIMaxPayloadViewBytes)

	// Settings of the spec whose feature is not built yet: setting one warns,
	// so an operator is not left expecting an effect.
	for _, key := range AgentTracingReservedEnv {
		if v, ok := p.get(key); ok && !(key == "AGENT_TRACING_PAYLOAD_BACKEND" && cfg.PayloadBackend == "db") {
			p.warn("%s=%s has no effect yet: the feature is planned for a later release", key, v)
		}
	}

	return cfg, p.warnings
}

// AgentTracingReservedEnv are the AGENT_TRACING_* settings parsed for the
// spec's configuration surface but not used yet: the filesystem payload
// backend (only db is built), the client-retry and delegation correlation
// rules, and the OTel export's per-span inline content cap. Each warns when
// set (bead jyx.110).
var AgentTracingReservedEnv = []string{
	"AGENT_TRACING_PAYLOAD_BACKEND",
	"AGENT_TRACING_RETRY_WINDOW",
	"AGENT_TRACING_DELEGATION_TOOL_PATTERNS",
	"AGENT_TRACING_EXPORT_MAX_SPAN_CONTENT_BYTES",
}

// atParser reads AGENT_TRACING_* values, keeping the default and recording
// a warning for anything invalid.
type atParser struct {
	getenv   func(string) string
	warnings []string
}

func (p *atParser) warn(format string, args ...interface{}) {
	p.warnings = append(p.warnings, fmt.Sprintf(format, args...))
}

func (p *atParser) get(key string) (string, bool) {
	v := strings.TrimSpace(p.getenv(key))
	return v, v != ""
}

func (p *atParser) boolean(key string, dst *bool) {
	if v, ok := p.get(key); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			p.warn("Invalid %s value %q; using default %t", key, v, *dst)
			return
		}
		*dst = b
	}
}

func (p *atParser) positiveInt(key string, dst *int) {
	if v, ok := p.get(key); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			p.warn("Invalid %s value %q (want a positive integer); using default %d", key, v, *dst)
			return
		}
		*dst = n
	}
}

func (p *atParser) positiveInt64(key string, dst *int64) {
	if v, ok := p.get(key); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			p.warn("Invalid %s value %q (want a positive integer); using default %d", key, v, *dst)
			return
		}
		*dst = n
	}
}

func (p *atParser) positiveDuration(key string, dst *time.Duration) {
	if v, ok := p.get(key); ok {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			p.warn("Invalid %s value %q (want a positive Go duration such as 30s); using default %s", key, v, *dst)
			return
		}
		*dst = d
	}
}

func (p *atParser) fraction(key string, dst *float64) {
	if v, ok := p.get(key); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			p.warn("Invalid %s value %q (want a number from 0 to 1); using default %g", key, v, *dst)
			return
		}
		*dst = f
	}
}

func (p *atParser) enum(key string, dst *string, allowed ...string) {
	if v, ok := p.get(key); ok {
		v = strings.ToLower(v)
		for _, a := range allowed {
			if v == a {
				*dst = v
				return
			}
		}
		p.warn("Invalid %s value %q (want one of %s); using default %s", key, v, strings.Join(allowed, ", "), *dst)
	}
}

// list reads a comma-separated list. The literal "none" sets an empty list.
func (p *atParser) list(key string, dst *[]string) {
	if v, ok := p.get(key); ok {
		if strings.EqualFold(v, "none") {
			*dst = []string{}
			return
		}
		*dst = splitCSVList(v)
	}
}

// regexList reads a list of regular expressions separated by newlines or
// ";;" (commas are common inside patterns). Invalid patterns are dropped
// with a warning.
func (p *atParser) regexList(key string, dst *[]string) {
	v, ok := p.get(key)
	if !ok {
		return
	}
	if strings.EqualFold(v, "none") {
		*dst = []string{}
		return
	}
	var out []string
	for _, line := range strings.FieldsFunc(strings.ReplaceAll(v, ";;", "\n"), func(r rune) bool { return r == '\n' }) {
		pat := strings.TrimSpace(line)
		if pat == "" {
			continue
		}
		if _, err := regexp.Compile(pat); err != nil {
			p.warn("Invalid pattern %q in %s: %v; ignoring it", pat, key, err)
			continue
		}
		out = append(out, pat)
	}
	*dst = out
}
