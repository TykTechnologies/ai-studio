package plugins

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
)

// The edge's pre-auth, auth and post-auth hooks set the request kind on the
// plugin context; it reaches a gRPC plugin only through these conversions.
func TestPluginContextConversion_CarriesRequestKind(t *testing.T) {
	in := &interfaces.PluginContext{
		RequestID: "req-1", LLMID: 5, LLMSlug: "jev", Vendor: "systemone",
		RequestKind: "decision", AppID: 7, Metadata: map[string]interface{}{"edge_id": "e1"},
	}
	pb := convertPluginContext(in)
	assert.Equal(t, "decision", pb.RequestKind)
	assert.Equal(t, "systemone", pb.Vendor)
	assert.Equal(t, "e1", pb.Metadata["edge_id"])

	back := convertPBPluginContext(pb)
	assert.Equal(t, "decision", back.RequestKind)
	assert.Equal(t, "systemone", back.Vendor)
	assert.Equal(t, uint(5), back.LLMID)

	// A chat request (and every context built before request kinds existed)
	// stays empty.
	assert.Equal(t, "", convertPluginContext(&interfaces.PluginContext{RequestID: "r"}).RequestKind)
	assert.Equal(t, "", convertPluginContext(nil).RequestKind)
}
