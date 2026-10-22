package providers

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginIDs(t *testing.T, p *GRPCProvider, llmID uint) []uint {
	t.Helper()
	plugins, err := p.GetPluginsForLLM(llmID)
	require.NoError(t, err)
	ids := make([]uint, 0, len(plugins))
	for _, pl := range plugins {
		ids = append(ids, pl.ID)
	}
	return ids
}

// An edge runs an LLM's plugins in the order set in Studio (the LLM's
// plugin_ids, which the hub sends by order_index), not in the snapshot's
// plugin-id order (midsommar-clean-701). Each LLM has its own order.
func TestGRPCProvider_GetPluginsForLLM_LLMOrder(t *testing.T) {
	p := &GRPCProvider{namespaceFilter: &DefaultNamespaceFilter{}}
	p.configCache = &pb.ConfigurationSnapshot{
		// The snapshot lists plugins by id.
		Plugins: []*pb.PluginConfig{
			{Id: 1, Name: "cache", IsActive: true, LlmIds: []uint32{10, 20}},
			{Id: 2, Name: "compressor", IsActive: true, LlmIds: []uint32{10, 20, 30}},
			{Id: 3, Name: "limiter", IsActive: true, LlmIds: []uint32{10, 30}},
			{Id: 4, Name: "inactive", IsActive: false, LlmIds: []uint32{10}},
		},
		Llms: []*pb.LLMConfig{
			{Id: 10, PluginIds: []uint32{3, 2, 1}},
			{Id: 20, PluginIds: []uint32{1, 2}},
			// A hub that sent no order: the snapshot's order stands.
			{Id: 30},
		},
	}

	assert.Equal(t, []uint{3, 2, 1}, pluginIDs(t, p, 10), "the reverse of their ids")
	assert.Equal(t, []uint{1, 2}, pluginIDs(t, p, 20))
	assert.Equal(t, []uint{2, 3}, pluginIDs(t, p, 30))
	assert.Empty(t, pluginIDs(t, p, 99))
}

// A plugin the LLM's list does not name goes after the ones it does.
func TestGRPCProvider_GetPluginsForLLM_UnlistedPluginLast(t *testing.T) {
	p := &GRPCProvider{namespaceFilter: &DefaultNamespaceFilter{}}
	p.configCache = &pb.ConfigurationSnapshot{
		Plugins: []*pb.PluginConfig{
			{Id: 1, IsActive: true, LlmIds: []uint32{10}},
			{Id: 2, IsActive: true, LlmIds: []uint32{10}},
			{Id: 3, IsActive: true, LlmIds: []uint32{10}},
		},
		Llms: []*pb.LLMConfig{{Id: 10, PluginIds: []uint32{3, 2}}},
	}
	assert.Equal(t, []uint{3, 2, 1}, pluginIDs(t, p, 10))
}
