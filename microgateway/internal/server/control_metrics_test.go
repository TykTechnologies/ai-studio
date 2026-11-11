package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// microgateway_control_connected is 1 for the endpoint whose stream is open
// and 0 for every other configured endpoint; all zeros while disconnected.
func TestControlLinkValues(t *testing.T) {
	endpoints := []string{"cp-a:50051", "cp-b:50051"}
	link := func(endpoint string, connected bool) func() (string, bool) {
		return func() (string, bool) { return endpoint, connected }
	}

	assert.Equal(t, map[string]float64{"cp-a:50051": 1, "cp-b:50051": 0}, controlLinkValues(endpoints, link("cp-a:50051", true)))
	assert.Equal(t, map[string]float64{"cp-a:50051": 0, "cp-b:50051": 1}, controlLinkValues(endpoints, link("cp-b:50051", true)))
	assert.Equal(t, map[string]float64{"cp-a:50051": 0, "cp-b:50051": 0}, controlLinkValues(endpoints, link("cp-b:50051", false)),
		"the endpoint tried last is not reported as connected")
}

// microgateway_config_source is 1 for the source in use; and
// microgateway_config_age_seconds is the time since the last contact with a
// control plane, 0 for an edge that never had one.
func TestConfigSourceMetricValues(t *testing.T) {
	now := time.Now()
	source := func(s string, contact time.Time) func() (string, time.Time) {
		return func() (string, time.Time) { return s, contact }
	}

	assert.Equal(t, map[string]float64{"control": 1, "last_known_good": 0}, configSourceValues(source("control", now)))
	assert.Equal(t, map[string]float64{"control": 0, "last_known_good": 1}, configSourceValues(source("last_known_good", now)))

	assert.InDelta(t, 90, configAgeSeconds(source("last_known_good", now.Add(-90*time.Second)), now), 0.001)
	assert.InDelta(t, 2, configAgeSeconds(source("control", now.Add(-2*time.Second)), now), 0.001)
	assert.Zero(t, configAgeSeconds(source("control", time.Time{}), now), "never had a control plane")
	assert.Zero(t, configAgeSeconds(source("control", now.Add(time.Minute)), now), "a clock that stepped back does not give a negative age")
}

// Every reason a pulse drops records for is a series from the start, at 0,
// so an alert on the counter does not wait for the first loss to exist.
func TestPulseDropMetricValues(t *testing.T) {
	assert.Equal(t,
		map[string]float64{"over_cap": 12, "expired": 0, "oversized": 1},
		pulseDropValues(map[string]uint64{"over_cap": 12, "expired": 0, "oversized": 1}))
}
