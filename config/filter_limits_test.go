package config

import (
	"testing"
	"time"
)

// Studio's configuration sets the tuning limits it lists; the environment
// is the fallback, and the only source for names it does not list.
func TestFilterLimit(t *testing.T) {
	t.Cleanup(ResetGlobalConfig)
	t.Setenv("FILTER_SCRIPT_TIMEOUT", " 3s ")
	t.Setenv("GUARDRAIL_VERDICT_CACHE_ENTRIES", "500")
	t.Setenv("FILTER_SCRIPT_ALLOW_OS", "false")

	ResetGlobalConfig()
	if got := FilterLimitDuration("FILTER_SCRIPT_TIMEOUT", time.Second); got != 3*time.Second {
		t.Errorf("no configuration: FILTER_SCRIPT_TIMEOUT = %v, want the environment's 3s", got)
	}

	Set(&AppConf{FilterLimits: map[string]string{
		"FILTER_SCRIPT_TIMEOUT":  "7",
		"FILTER_SCRIPT_ALLOW_OS": "true",
	}})
	if got := FilterLimitDuration("FILTER_SCRIPT_TIMEOUT", time.Second); got != 7*time.Second {
		t.Errorf("configured bare integer: %v, want 7s", got)
	}
	if got := FilterLimitInt64("GUARDRAIL_VERDICT_CACHE_ENTRIES", 1); got != 500 {
		t.Errorf("not configured: %d, want the environment's 500", got)
	}
	if got := FilterLimit("FILTER_SCRIPT_ALLOW_OS"); got != "false" {
		t.Errorf("a name not in FilterLimitNames took the configured %q", got)
	}

	t.Setenv("GUARDRAIL_VERDICT_CACHE_TTL", "soon")
	if got := FilterLimitDuration("GUARDRAIL_VERDICT_CACHE_TTL", time.Minute); got != time.Minute {
		t.Errorf("unparseable duration: %v, want the default", got)
	}
	t.Setenv("GUARDRAIL_LOCAL_SCAN_SLOTS", "many")
	if got := FilterLimitInt64("GUARDRAIL_LOCAL_SCAN_SLOTS", 4); got != 4 {
		t.Errorf("unparseable integer: %d, want the default", got)
	}
}
