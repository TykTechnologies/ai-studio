package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestAgentTracingConfigDefaults(t *testing.T) {
	cfg, warnings := parseAgentTracingConfig(envMap(nil))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !reflect.DeepEqual(cfg, DefaultAgentTracingConfig()) {
		t.Fatalf("defaults differ from DefaultAgentTracingConfig")
	}
	// Spot-check the §11 values (as amended by D5 and Q11).
	if cfg.Enabled || cfg.ContentDefaultMode != "off" || cfg.ContentMaxRequestOverride != "metadata" ||
		cfg.QueueMaxEvents != 10000 || cfg.QueueMaxBytes != 67108864 || cfg.MaxStreamBufferBytes != 1048576 ||
		cfg.PayloadMaxPartBytes != 262144 || cfg.PayloadMaxSpanBytes != 1048576 ||
		cfg.TurnIdleTimeout != 5*time.Minute || cfg.SessionIdleTimeout != 30*time.Minute ||
		cfg.ToolMatchWindow != 300*time.Second || cfg.MetadataRetentionDays != 30 || cfg.PayloadRetentionDays != 7 ||
		cfg.CorrelationKeyTTL != 24*time.Hour || cfg.StoreSampleRate != 1.0 || cfg.ExportContent != "none" ||
		cfg.ExportSemconvProfile != "genai-1.37" || !cfg.PropagateLLMProviders || cfg.ForwardTykHeaders ||
		!reflect.DeepEqual(cfg.BaggageAllowlist, []string{"session.id"}) || len(cfg.HarnessSessionHeaders) != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestAgentTracingConfigParse(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		check    func(t *testing.T, c AgentTracingConfig)
		warnings int
	}{
		{
			name: "valid overrides",
			env: map[string]string{
				"AGENT_TRACING_ENABLED":                 "true",
				"AGENT_TRACING_KEY":                     "k",
				"AGENT_TRACING_QUEUE_MAX_EVENTS":        "500",
				"AGENT_TRACING_CONTENT_DEFAULT_MODE":    "Redacted",
				"AGENT_TRACING_TURN_IDLE_TIMEOUT":       "90s",
				"AGENT_TRACING_STORE_SAMPLE_RATE":       "0.25",
				"AGENT_TRACING_COMPAT_HEADERS":          "helicone, Portkey",
				"AGENT_TRACING_HARNESS_SESSION_HEADERS": "x-claude-code-session-id",
				"AGENT_TRACING_PAYLOAD_BACKEND":         "postgres",
				"AGENT_TRACING_EXPORTERS_FILE":          "/etc/tyk/exporters.yaml",
				"AGENT_TRACING_PROPAGATE_LLM_PROVIDERS": "false",
			},
			check: func(t *testing.T, c AgentTracingConfig) {
				if !c.Enabled || c.Key != "k" || c.QueueMaxEvents != 500 || c.ContentDefaultMode != "redacted" ||
					c.TurnIdleTimeout != 90*time.Second || c.StoreSampleRate != 0.25 ||
					!reflect.DeepEqual(c.CompatHeaders, []string{"helicone", "portkey"}) ||
					!reflect.DeepEqual(c.HarnessSessionHeaders, []string{"x-claude-code-session-id"}) ||
					c.PayloadBackend != "db" || c.ExportersFile != "/etc/tyk/exporters.yaml" || c.PropagateLLMProviders {
					t.Fatalf("overrides not applied: %+v", c)
				}
			},
		},
		{
			name: "invalid values fall back to defaults",
			env: map[string]string{
				"AGENT_TRACING_ENABLED":              "maybe",
				"AGENT_TRACING_QUEUE_MAX_EVENTS":     "-1",
				"AGENT_TRACING_QUEUE_MAX_BYTES":      "lots",
				"AGENT_TRACING_CONTENT_DEFAULT_MODE": "everything",
				"AGENT_TRACING_TURN_IDLE_TIMEOUT":    "5 minutes",
				"AGENT_TRACING_SESSION_IDLE_TIMEOUT": "-1m",
				"AGENT_TRACING_STORE_SAMPLE_RATE":    "1.5",
				"AGENT_TRACING_MIN_LINK_CONFIDENCE":  "certain",
				"AGENT_TRACING_EXPORT_CONTENT":       "all",
			},
			warnings: 9,
			check: func(t *testing.T, c AgentTracingConfig) {
				d := DefaultAgentTracingConfig()
				if c.Enabled || c.QueueMaxEvents != d.QueueMaxEvents || c.QueueMaxBytes != d.QueueMaxBytes ||
					c.ContentDefaultMode != "off" || c.TurnIdleTimeout != d.TurnIdleTimeout ||
					c.SessionIdleTimeout != d.SessionIdleTimeout || c.StoreSampleRate != 1.0 ||
					c.MinLinkConfidence != "weak" || c.ExportContent != "none" {
					t.Fatalf("invalid values were not replaced by defaults: %+v", c)
				}
			},
		},
		{
			name:     "payload retention capped at metadata retention",
			env:      map[string]string{"AGENT_TRACING_METADATA_RETENTION_DAYS": "5", "AGENT_TRACING_PAYLOAD_RETENTION_DAYS": "9"},
			warnings: 1,
			check: func(t *testing.T, c AgentTracingConfig) {
				if c.MetadataRetentionDays != 5 || c.PayloadRetentionDays != 5 {
					t.Fatalf("got metadata %d payload %d", c.MetadataRetentionDays, c.PayloadRetentionDays)
				}
			},
		},
		{
			name:     "part cap capped at span cap",
			env:      map[string]string{"AGENT_TRACING_PAYLOAD_MAX_PART_BYTES": "2000000"},
			warnings: 1,
			check: func(t *testing.T, c AgentTracingConfig) {
				if c.PayloadMaxPartBytes != c.PayloadMaxSpanBytes {
					t.Fatalf("part cap %d not capped to %d", c.PayloadMaxPartBytes, c.PayloadMaxSpanBytes)
				}
			},
		},
		{
			name: "none empties a list; regex list drops bad patterns",
			env: map[string]string{
				"AGENT_TRACING_COMPAT_HEADERS":           "none",
				"AGENT_TRACING_DELEGATION_TOOL_PATTERNS": "^task$;;^(unclosed;;^spawn_.*$",
			},
			warnings: 1,
			check: func(t *testing.T, c AgentTracingConfig) {
				if len(c.CompatHeaders) != 0 || c.CompatHeaders == nil {
					t.Fatalf("compat headers not emptied: %#v", c.CompatHeaders)
				}
				if !reflect.DeepEqual(c.DelegationToolPatterns, []string{"^task$", "^spawn_.*$"}) {
					t.Fatalf("patterns: %#v", c.DelegationToolPatterns)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, warnings := parseAgentTracingConfig(envMap(tt.env))
			if len(warnings) != tt.warnings {
				t.Fatalf("want %d warnings, got %d: %s", tt.warnings, len(warnings), strings.Join(warnings, " | "))
			}
			tt.check(t, cfg)
		})
	}
}

func TestAgentTracingContentModeRank(t *testing.T) {
	if AgentTracingContentModeRank("off") != 0 || AgentTracingContentModeRank("full") != 4 ||
		AgentTracingContentModeRank("hashed") <= AgentTracingContentModeRank("metadata") ||
		AgentTracingContentModeRank("bogus") != -1 {
		t.Fatal("content mode ordering broken")
	}
}
