package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What an edge keeps for the control plane through an outage, as it is
// today (bead .36 documents it; changing it is a decision of its own).
//
// Records wait in memory. While pulses fail, whatever the reason, the edge
// keeps at most max_buffer_size of them: each time a pulse is given up
// (after its retries) the batch it carried and everything buffered since
// are trimmed to that many, oldest first. When a control plane answers
// again it receives those newest records and nothing older: the edge does
// not go back to the copy of the analytics in its own database. What was
// trimmed is counted (microgateway_analytics_pulse_dropped_records_total,
// reason over_cap) and is missing from the control plane's analytics and
// from the spend it computes, for good.
func TestAnalyticsPulse_OutageKeepsOnlyTheNewestMaxBufferSizeRecords(t *testing.T) {
	const cap = 30
	hub := &fakeSyncClient{failures: 1 << 30}
	p := newRetryPlugin(hub, PulsePluginConfig{MaxBufferSize: cap, MaxRetries: 1})
	before := PulseDroppedRecords()["over_cap"]

	// The outage: 10 rounds of 100 requests' events, a pulse attempted
	// after each (as the timer or a full buffer would).
	var ids []string
	for round := 0; round < 10; round++ {
		ids = append(ids, bufferEvents(p, "r"+string(rune('a'+round)), 100, 1)...)
		p.sendPulse()
		require.LessOrEqual(t, p.totalBufferedLocked(), cap, "after a failed pulse the edge holds at most max_buffer_size records")
	}
	assert.EqualValues(t, 1000-cap, PulseDroppedRecords()["over_cap"]-before, "everything but the newest max_buffer_size records is given up, and counted")

	// A control plane answers again.
	hub.mu.Lock()
	hub.failures = 0
	hub.pulses = nil
	hub.mu.Unlock()
	p.sendPulse()
	p.sendPulse()

	var delivered []string
	hub.mu.Lock()
	for _, pulse := range hub.pulses {
		for _, ev := range pulse.AnalyticsEvents {
			delivered = append(delivered, ev.RequestId)
		}
	}
	hub.mu.Unlock()
	assert.Equal(t, ids[len(ids)-cap:], delivered, "the control plane gets the newest max_buffer_size records, once, and nothing of the 970 before them")
	assert.Zero(t, p.totalBufferedLocked())
}
