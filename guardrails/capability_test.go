package guardrails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapability(t *testing.T) {
	for _, s := range []string{"", "read_only", "read_tools", "acting"} {
		c, err := ParseCapability(s)
		assert.NoError(t, err, s)
		assert.Equal(t, Capability(s), c)
	}
	_, err := ParseCapability("admin")
	assert.Error(t, err)

	assert.Equal(t, CapabilityActing, CapabilityUnattested.Effective(), "unattested is the strictest")
	assert.Equal(t, CapabilityActing, Capability("bogus").Effective())
	assert.Equal(t, CapabilityReadOnly, CapabilityReadOnly.Effective())
	assert.Equal(t, CapabilityReadTools, CapabilityReadTools.Effective())
}
