package services

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
)

// licenceStub is a licensing service whose validity a test flips.
type licenceStub struct {
	licensing.Service
	valid atomic.Bool
}

func (l *licenceStub) IsValid() bool { return l.valid.Load() }

// Capture stops while the licence is not valid and resumes once it is again,
// for a service the start gate started; otherwise nothing changes (jyx.56).
func TestSyncAgentTracingLicence(t *testing.T) {
	t.Cleanup(func() { agenttrace.SetEnabled(false) })
	lic := &licenceStub{}
	s := &Service{LicensingService: lic}

	t.Run("not started: capture untouched", func(t *testing.T) {
		agenttrace.SetEnabled(false)
		lic.valid.Store(true)
		s.SyncAgentTracingLicence()
		assert.False(t, agenttrace.Enabled(), "a gate that never opened is not opened by a licence")
	})

	s.agentTracingStarted.Store(true)
	agenttrace.SetEnabled(true)

	lic.valid.Store(false)
	s.SyncAgentTracingLicence()
	assert.False(t, agenttrace.Enabled(), "an invalid licence stops capture")

	s.SyncAgentTracingLicence()
	assert.False(t, agenttrace.Enabled())

	lic.valid.Store(true)
	s.SyncAgentTracingLicence()
	assert.True(t, agenttrace.Enabled(), "a renewed licence resumes capture")

	t.Run("no licensing service", func(t *testing.T) {
		bare := &Service{}
		bare.agentTracingStarted.Store(true)
		agenttrace.SetEnabled(true)
		bare.SyncAgentTracingLicence()
		assert.True(t, agenttrace.Enabled())
	})
}
