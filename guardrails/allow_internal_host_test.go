package guardrails

import "testing"

// A guardrail's "Allow internal host" switch (w4e) exempts the hosts of its
// connection's URL values only, never a key that happens to look like a
// host, and travels to edges in the config.
func TestConfig_AllowInternalHost(t *testing.T) {
	cfg := Config{Provider: ProviderHTTP, AllowInternalHost: true, Connection: map[string]string{
		"endpoint": "http://classifier.internal:8080/v1", "api_key": "redis",
	}}
	e := cfg.Exemption()
	if !e.Has("classifier.internal") {
		t.Error("the endpoint's host is not exempt")
	}
	if e.Has("redis") {
		t.Error("a connection key was taken for a host")
	}
	cfg.AllowInternalHost = false
	if cfg.Exemption().Has("classifier.internal") {
		t.Error("the switch off exempts the endpoint")
	}

	raw := map[string]any{
		"provider":            ProviderBuiltin,
		"allow_internal_host": true,
		"detectors":           []any{map[string]any{"name": CategoryInjection}},
		"on_detect":           ActionBlock,
	}
	js, err := ConfigJSONForEdge(raw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := ConfigFromJSON(js)
	parsed, err := ParseConfig(m)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.AllowInternalHost {
		t.Errorf("the switch did not travel to the edge: %s", js)
	}
}
