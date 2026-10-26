package fipsinfo

import (
	"crypto/fips140"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentMatchesTheModule(t *testing.T) {
	s := Current()
	assert.Equal(t, fips140.Enabled(), s.Enabled)
	assert.Equal(t, fips140.Enforced(), s.Enforced)
	assert.Equal(t, fips140.Version(), s.Version)
	if !fips140.Enabled() {
		assert.False(t, s.Enforced, "a standard run is neither enabled nor enforced")
	}
}

func TestMetricsRoundTrip(t *testing.T) {
	for _, s := range []Status{
		{Enabled: true, Version: "v1.0.0-c2097c7c", Enforced: true},
		{Enabled: true, Version: "v1.0.0-c2097c7c"},
		{Version: "latest"},
		{Enabled: true, Version: strings.Repeat("v", 64)},
	} {
		got, ok := FromMetrics(s.Metrics())
		assert.True(t, ok)
		assert.Equal(t, s, got)
	}
}

func TestFromMetrics_UnknownWhenAbsent(t *testing.T) {
	for _, m := range []map[string]string{
		nil,
		{},
		{"cpu": "1"},
		{MetricEnabled: "yes", MetricEnforced: "false"},
		{MetricEnabled: "true"},
		// The hub stores and shows the version: refuse long or label-like ones.
		{MetricEnabled: "true", MetricEnforced: "false", MetricVersion: strings.Repeat("v", 65)},
		{MetricEnabled: "true", MetricEnforced: "false", MetricVersion: "v1.0.0) — verified by Tyk"},
		{MetricEnabled: "true", MetricEnforced: "false", MetricVersion: "v1.0.0\n<b>"},
	} {
		_, ok := FromMetrics(m)
		assert.False(t, ok, "%v", m)
	}
}
