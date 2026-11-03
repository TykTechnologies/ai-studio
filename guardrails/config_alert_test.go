package guardrails

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/internal/v222"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertConfig(mutate func(m map[string]any)) map[string]any {
	m := map[string]any{
		"provider":  ProviderBuiltin,
		"detectors": []any{map[string]any{"name": CategoryInjection}},
		"on_detect": ActionAlert,
		"alert":     map[string]any{"event": "visor-pr-flag"},
	}
	if mutate != nil {
		mutate(m)
	}
	return m
}

func normalizeRaw(t *testing.T, raw map[string]any, response bool) (Config, error) {
	t.Helper()
	cfg, err := ParseConfig(raw)
	require.NoError(t, err)
	return Normalize(cfg, response)
}

func TestAlertAction_Validation(t *testing.T) {
	cfg, err := normalizeRaw(t, alertConfig(nil), false)
	require.NoError(t, err)
	assert.Equal(t, ActionAlert, cfg.OnDetect)
	assert.Equal(t, "visor-pr-flag", cfg.AlertFor(KindText).Event)
	assert.Equal(t, []string{"visor-pr-flag"}, cfg.AlertEvents())

	_, err = normalizeRaw(t, alertConfig(nil), true)
	assert.NoError(t, err, "responses may alert")

	refused := map[string]func(m map[string]any){
		"no event":      func(m map[string]any) { delete(m, "alert") },
		"empty event":   func(m map[string]any) { m["alert"] = map[string]any{"event": ""} },
		"uppercase":     func(m map[string]any) { m["alert"] = map[string]any{"event": "Visor"} },
		"one character": func(m map[string]any) { m["alert"] = map[string]any{"event": "v"} },
		"leading dash":  func(m map[string]any) { m["alert"] = map[string]any{"event": "-visor"} },
		"dot":           func(m map[string]any) { m["alert"] = map[string]any{"event": "visor.pr"} },
		"too long": func(m map[string]any) {
			m["alert"] = map[string]any{"event": "a0123456789012345678901234567890123456789012345678901234567890123"}
		},
		"excerpt no evidence": func(m map[string]any) { m["alert"] = map[string]any{"event": "visor", "include_excerpt": true} },
		"alert without action": func(m map[string]any) {
			m["on_detect"] = ActionLog
		},
		"duplicate within the filter": func(m map[string]any) {
			m["tool_results"] = map[string]any{"on_detect": ActionAlert, "alert": map[string]any{"event": "visor-pr-flag"}}
		},
		"tool alert without event": func(m map[string]any) {
			m["tool_calls"] = map[string]any{"scan": true, "on_detect": ActionAlert}
		},
	}
	for name, mutate := range refused {
		_, err := normalizeRaw(t, alertConfig(mutate), false)
		assert.ErrorIs(t, err, ErrInvalidConfig, name)
	}

	// include_excerpt with evidence.excerpt on is fine.
	_, err = normalizeRaw(t, alertConfig(func(m map[string]any) {
		m["alert"] = map[string]any{"event": "visor", "include_excerpt": true}
		m["evidence"] = map[string]any{"excerpt": true}
	}), false)
	assert.NoError(t, err)
}

func TestAlertAction_PerChannelAndInheritance(t *testing.T) {
	cfg, err := normalizeRaw(t, alertConfig(func(m map[string]any) {
		m["tool_results"] = map[string]any{"on_detect": ActionAlert, "alert": map[string]any{"event": "tool-flag"}}
		m["tool_calls"] = map[string]any{"scan": true}
		m["tool_definitions"] = map[string]any{"scan": true}
	}), false)
	require.NoError(t, err)
	assert.Equal(t, "tool-flag", cfg.AlertFor(KindToolResult).Event)
	assert.Equal(t, ActionAlert, cfg.ActionFor(KindToolCall), "tool calls inherit the filter's alert")
	assert.Equal(t, "visor-pr-flag", cfg.AlertFor(KindToolCall).Event)
	assert.Equal(t, ActionAlert, cfg.ActionFor(KindToolDefinition))
	assert.ElementsMatch(t, []string{"visor-pr-flag", "tool-flag"}, cfg.AlertEvents())

	blocking, err := normalizeRaw(t, alertConfig(func(m map[string]any) {
		m["on_detect"] = ActionBlock
		delete(m, "alert")
		m["tool_results"] = map[string]any{"on_detect": ActionAlert, "alert": map[string]any{"event": "tool-flag"}}
	}), false)
	require.NoError(t, err)
	assert.Nil(t, blocking.AlertFor(KindText))
	assert.Equal(t, "tool-flag", blocking.AlertFor(KindToolResult).Event)
}

// A shadow filter may alert: alert is log for the traffic.
func TestAlertAction_ShadowAllowed(t *testing.T) {
	_, err := normalizeRaw(t, alertConfig(func(m map[string]any) { m["shadow"] = true }), false)
	assert.NoError(t, err)
	_, err = normalizeRaw(t, alertConfig(func(m map[string]any) {
		m["shadow"] = true
		m["tool_results"] = map[string]any{"on_detect": ActionAlert, "alert": map[string]any{"event": "tool-flag"}}
	}), false)
	assert.NoError(t, err)
}

// An edge older than alert logs, which is what alert does to the traffic.
func TestAlertAction_OlderEdgeLogs(t *testing.T) {
	js, err := ConfigJSONForEdge(alertConfig(nil), nil, nil)
	require.NoError(t, err)
	oldRaw, err := v222.ConfigFromJSON(js)
	require.NoError(t, err)
	old, err := v222.ParseConfig(oldRaw)
	require.NoError(t, err)
	old, err = v222.Normalize(old, false)
	require.NoError(t, err)
	assert.Equal(t, v222.ActionLog, old.OnDetect)

	// A current edge reads alert back.
	curRaw, err := ConfigFromJSON(js)
	require.NoError(t, err)
	cur, err := ParseConfig(curRaw)
	require.NoError(t, err)
	assert.Equal(t, ActionAlert, cur.OnDetect)
	assert.Equal(t, "visor-pr-flag", cur.Alert.Event)
}

func TestLogIfAlert(t *testing.T) {
	assert.Equal(t, ActionLog, LogIfAlert(ActionAlert))
	assert.Equal(t, ActionBlock, LogIfAlert(ActionBlock))
	assert.Equal(t, ActionRank(ActionLog), ActionRank(ActionAlert))
}
