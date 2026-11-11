package server

import (
	"testing"

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
