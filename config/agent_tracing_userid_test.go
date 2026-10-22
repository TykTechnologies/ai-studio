package config

import "testing"

// user_id_mode replaces include_user_id (jyx.173): hashed by default; an
// explicit legacy boolean still applies when the mode is not set (true
// keeps raw, false turns it off); the mode wins when both are set; an
// invalid mode keeps the default with a warning.
func TestAgentTracingUserIDMode(t *testing.T) {
	cases := []struct {
		env      map[string]string
		want     string
		warnings int
	}{
		{map[string]string{}, "hashed", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_INCLUDE_USER_ID": "true"}, "raw", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_INCLUDE_USER_ID": "false"}, "off", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_USER_ID_MODE": "raw"}, "raw", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_USER_ID_MODE": "off", "AGENT_TRACING_EXPORT_INCLUDE_USER_ID": "true"}, "off", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_USER_ID_MODE": "HASHED"}, "hashed", 0},
		{map[string]string{"AGENT_TRACING_EXPORT_USER_ID_MODE": "plain"}, "hashed", 1},
		// jyx.178: an unparsable legacy value keeps hashed, with a warning.
		{map[string]string{"AGENT_TRACING_EXPORT_INCLUDE_USER_ID": "yes"}, "hashed", 1},
	}
	for _, c := range cases {
		cfg, warnings := parseAgentTracingConfig(func(k string) string { return c.env[k] })
		if cfg.ExportUserIDMode != c.want {
			t.Errorf("%v: mode %q, want %q", c.env, cfg.ExportUserIDMode, c.want)
		}
		if len(warnings) != c.warnings {
			t.Errorf("%v: %d warnings %v, want %d", c.env, len(warnings), warnings, c.warnings)
		}
	}
}
