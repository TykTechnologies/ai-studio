package guardrails

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConfigJSONForEdge renders a stored guardrail config for the edge snapshot.
// Connection values are passed through resolve so `$SECRET/` and `$ENV/`
// references become the values an edge, which has no access to the hub's
// secret store, can use. This mirrors how LLM API keys reach edges.
func ConfigJSONForEdge(raw map[string]any, resolve func(string) string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return "", err
	}
	if resolve != nil {
		for k, v := range cfg.Connection {
			cfg.Connection[k] = resolve(v)
		}
	}
	// An edge older than this hub rejects an action it does not know. Send
	// it block instead, the strongest action every edge applies, and the
	// real action where only newer edges look. New keys elsewhere in the
	// config (quarantine, tool_results, budgets) are ignored by old edges,
	// which then behave as they always have.
	if !legacyActions[cfg.OnDetect] {
		cfg.OnDetectExt = cfg.OnDetect
		cfg.OnDetect = ActionBlock
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ConfigFromJSON decodes the edge snapshot form back into the stored map
// shape. An empty string is an absent config.
func ConfigFromJSON(s string) (map[string]any, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("guardrail config: %w", err)
	}
	return m, nil
}
