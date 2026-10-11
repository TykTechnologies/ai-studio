package api

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
)

// Response plugins on the edge receive the vendor and request kind. The
// vendor was always sent empty before ("set from LLM context if needed"), so
// a response plugin that keyed on it now sees the LLM's vendor.
func TestConvertPluginContextToPB_CarriesVendorAndRequestKind(t *testing.T) {
	pb := convertPluginContextToPB(&proxy.PluginContext{
		RequestID: "req-1", LLMSlug: "jev", LLMID: 5, AppID: 7,
		Metadata: map[string]string{"vendor": "systemone"},
		Vendor:   "systemone", RequestKind: "decision",
	})
	assert.Equal(t, "systemone", pb.Vendor)
	assert.Equal(t, "decision", pb.RequestKind)
	assert.Equal(t, "systemone", pb.Metadata["vendor"])
	assert.Equal(t, uint32(5), pb.LlmId)

	assert.Equal(t, "", convertPluginContextToPB(&proxy.PluginContext{RequestID: "r"}).RequestKind)
	assert.NotNil(t, convertPluginContextToPB(nil))
}
