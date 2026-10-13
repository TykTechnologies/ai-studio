package snapshotguard

import (
	"strings"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSanitize_CleanSnapshotIsReturnedAsIs(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Llms:  []*pb.LLMConfig{{Id: 1, Slug: "a"}, {Id: 2, Slug: "b"}},
		Tools: []*pb.ToolConfig{{Id: 1, Slug: "a"}, {Id: 2, Slug: "b"}},
		ModelRouters: []*pb.ModelRouterConfig{
			// The same slug in two namespaces is two keys on the edge.
			{Id: 1, Slug: "r", Namespace: ""}, {Id: 2, Slug: "r", Namespace: "eu"},
		},
	}
	out, left := Sanitize(in)
	assert.Same(t, in, out)
	assert.Empty(t, left)
}

// Two tools with one slug: the lowest ID keeps it, the other is left out
// with its App grants, and the rest of the snapshot is untouched.
func TestSanitize_DuplicateToolSlug(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Tools: []*pb.ToolConfig{
			{Id: 3, Slug: "page-fetcher", FilterIds: []uint32{7}},
			{Id: 2, Slug: "page-fetcher", FilterIds: []uint32{8}},
			{Id: 4, Slug: "other"},
		},
		Apps: []*pb.AppConfig{
			{Id: 10, ToolIds: []uint32{2, 3, 4}},
			{Id: 11, ToolIds: []uint32{3}},
		},
		Filters: []*pb.FilterConfig{{Id: 7}, {Id: 8}},
		Llms:    []*pb.LLMConfig{{Id: 1, Slug: "gpt"}},
	}
	before := proto.Clone(in)

	out, left := Sanitize(in)
	require.Len(t, left, 1)
	assert.Equal(t, Entry{Kind: KindTool, ID: 3, Key: "page-fetcher", KeptID: 2}, left[0])
	assert.Equal(t, `tool 3 left out: slug "page-fetcher" is already used by tool 2`, left[0].String())

	var toolIDs []uint32
	for _, tool := range out.Tools {
		toolIDs = append(toolIDs, tool.Id)
	}
	assert.Equal(t, []uint32{2, 4}, toolIDs)
	assert.Equal(t, []uint32{2, 4}, out.Apps[0].ToolIds)
	assert.Empty(t, out.Apps[1].ToolIds)
	assert.Len(t, out.Filters, 2, "filters are never left out")
	assert.Len(t, out.Llms, 1)
	assert.True(t, proto.Equal(before, in), "the input snapshot was changed")
}

// An LLM left out for its slug takes its grants and attachments with it;
// the LLM that keeps the slug keeps every filter and plugin, and no filter
// or plugin is ever left out.
func TestSanitize_DuplicateLLMSlugKeepsProtections(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Llms: []*pb.LLMConfig{
			{Id: 5, Slug: "gpt-4", FilterIds: []uint32{1}, PluginIds: []uint32{30}},
			{Id: 9, Slug: "gpt-4", FilterIds: []uint32{2}, PluginIds: []uint32{31}},
		},
		Apps: []*pb.AppConfig{
			{Id: 10, LlmIds: []uint32{5, 9}},
			{Id: 11, LlmIds: []uint32{9}},
		},
		Filters: []*pb.FilterConfig{
			{Id: 1, LlmIds: []uint32{5}},
			{Id: 2, LlmIds: []uint32{9, 5}},
		},
		Plugins: []*pb.PluginConfig{
			{Id: 30, LlmIds: []uint32{5}},
			{Id: 31, LlmIds: []uint32{9}},
		},
		ModelRouters: []*pb.ModelRouterConfig{{
			Id: 1, Slug: "router",
			Pools: []*pb.ModelPoolConfig{{Id: 1, Vendors: []*pb.PoolVendorConfig{
				{Id: 1, LlmId: 9, LlmSlug: "gpt-4"},
				{Id: 2, LlmId: 5, LlmSlug: "gpt-4"},
			}}},
		}},
	}

	out, left := Sanitize(in)
	require.Len(t, left, 1)
	assert.Equal(t, Entry{Kind: KindLLM, ID: 9, Key: "gpt-4", KeptID: 5}, left[0])

	require.Len(t, out.Llms, 1)
	kept := out.Llms[0]
	assert.Equal(t, uint32(5), kept.Id)
	assert.Equal(t, []uint32{1}, kept.FilterIds, "the kept LLM's filters")
	assert.Equal(t, []uint32{30}, kept.PluginIds, "the kept LLM's plugins")

	assert.Len(t, out.Filters, 2, "filters are never left out")
	assert.Len(t, out.Plugins, 2, "plugins are never left out")
	assert.Equal(t, []uint32{5}, out.Filters[0].LlmIds)
	assert.Equal(t, []uint32{5}, out.Filters[1].LlmIds)
	assert.Equal(t, []uint32{5}, out.Plugins[0].LlmIds)
	assert.Empty(t, out.Plugins[1].LlmIds)

	assert.Equal(t, []uint32{5}, out.Apps[0].LlmIds)
	assert.Empty(t, out.Apps[1].LlmIds, "an App granted only the left-out LLM loses it, it does not reach the one that kept the slug")

	vendors := out.ModelRouters[0].Pools[0].Vendors
	require.Len(t, vendors, 1, "a router vendor of the left-out LLM would route to the LLM that kept the slug")
	assert.Equal(t, uint32(5), vendors[0].LlmId)
}

func TestSanitize_OtherUniqueKeys(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		ModelPrices: []*pb.ModelPriceConfig{
			{Id: 1, ModelName: "gpt-4", Vendor: "openai"},
			{Id: 2, ModelName: "gpt-4", Vendor: "openai"},
			{Id: 3, ModelName: "gpt-4", Vendor: "azure"},
		},
		ModelRouters: []*pb.ModelRouterConfig{
			{Id: 1, Slug: "r", Namespace: "eu"}, {Id: 2, Slug: "r", Namespace: "eu"},
		},
		SemanticRouters: []*pb.SemanticRouterConfig{
			{Id: 1, Slug: "s"}, {Id: 2, Slug: "s"},
		},
		Apps: []*pb.AppConfig{{Id: 10, ModelRouterIds: []uint32{1, 2}, SemanticRouterIds: []uint32{2}}},
		OauthClients: []*pb.OAuthClientConfig{
			{Id: 1, ClientId: "abc"}, {Id: 2, ClientId: "abc"},
		},
		AccessTokens: []*pb.AccessTokenConfig{
			{Id: 1, TokenHash: "h"}, {Id: 2, TokenHash: "h"},
		},
	}
	out, left := Sanitize(in)
	assert.Equal(t, []string{
		`model price 2 left out: model and vendor "gpt-4 / openai" is already used by model price 1`,
		`model router 2 left out: slug "r (namespace eu)" is already used by model router 1`,
		`semantic router 2 left out: slug "s" is already used by semantic router 1`,
		`OAuth client 2 left out: client ID "abc" is already used by OAuth client 1`,
		`access token 2 left out: its token hash is already used by access token 1`,
	}, Strings(left))
	for _, line := range Strings(left) {
		assert.False(t, strings.Contains(line, `"h"`), "a token hash is never reported")
	}
	for _, e := range left {
		if e.Kind == KindAccessToken {
			assert.Empty(t, e.Key, "a token hash is never reported")
		}
	}
	assert.Len(t, out.ModelPrices, 2)
	assert.Len(t, out.ModelRouters, 1)
	assert.Len(t, out.SemanticRouters, 1)
	assert.Len(t, out.OauthClients, 1)
	assert.Len(t, out.AccessTokens, 1)
	assert.Equal(t, []uint32{1}, out.Apps[0].ModelRouterIds)
	assert.Empty(t, out.Apps[0].SemanticRouterIds)
}

// The same row sent twice is stored once; references to it stay.
func TestSanitize_SameRowTwice(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Llms:    []*pb.LLMConfig{{Id: 1, Slug: "l"}, {Id: 1, Slug: "l"}},
		Tools:   []*pb.ToolConfig{{Id: 2, Slug: "t"}, {Id: 2, Slug: "t"}},
		Filters: []*pb.FilterConfig{{Id: 5, LlmIds: []uint32{1}}},
		Apps:    []*pb.AppConfig{{Id: 10, LlmIds: []uint32{1}, ToolIds: []uint32{2}}},
	}
	out, left := Sanitize(in)
	assert.Equal(t, []string{"LLM 1 left out: it appears more than once", "tool 2 left out: it appears more than once"}, Strings(left))
	assert.Len(t, out.Llms, 1)
	assert.Len(t, out.Tools, 1)
	assert.Equal(t, []uint32{1}, out.Apps[0].LlmIds, "the grant to the stored row stays")
	assert.Equal(t, []uint32{2}, out.Apps[0].ToolIds, "the grant to the stored row stays")
	assert.Equal(t, []uint32{1}, out.Filters[0].LlmIds)
}

// An LLM or tool attached to a filter or plugin the snapshot does not carry
// is left out, so it never runs without it; the others are kept.
func TestSanitize_MissingFilterOrPluginLeavesTheEntryOut(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Llms: []*pb.LLMConfig{
			{Id: 1, Slug: "guarded", FilterIds: []uint32{10}, PluginIds: []uint32{20}},
			{Id: 2, Slug: "no-filter", FilterIds: []uint32{10, 11}},
			{Id: 3, Slug: "no-plugin", PluginIds: []uint32{21}},
		},
		Tools: []*pb.ToolConfig{
			{Id: 4, Slug: "guarded-tool", FilterIds: []uint32{10}},
			{Id: 5, Slug: "bare-tool", FilterIds: []uint32{12}},
		},
		Filters: []*pb.FilterConfig{{Id: 10}},
		Plugins: []*pb.PluginConfig{{Id: 20}},
		Apps:    []*pb.AppConfig{{Id: 9, LlmIds: []uint32{1, 2, 3}, ToolIds: []uint32{4, 5}}},
	}
	out, left := Sanitize(in)
	assert.Equal(t, []string{
		"LLM 2 left out: its filter 11 is not in the configuration, and it does not run without it",
		"LLM 3 left out: its plugin 21 is not in the configuration, and it does not run without it",
		"tool 5 left out: its filter 12 is not in the configuration, and it does not run without it",
	}, Strings(left))
	require.Len(t, out.Llms, 1)
	assert.Equal(t, uint32(1), out.Llms[0].Id)
	require.Len(t, out.Tools, 1)
	assert.Equal(t, uint32(4), out.Tools[0].Id)
	assert.Equal(t, []uint32{1}, out.Apps[0].LlmIds)
	assert.Equal(t, []uint32{4}, out.Apps[0].ToolIds)
}

// An endpoint whose auth plugin the snapshot does not carry is left out
// rather than fall back to App keys. A plugin whose own endpoint's auth
// plugin is missing is left out, and so is every LLM that runs it.
func TestSanitize_MissingAuthPluginFailsClosed(t *testing.T) {
	in := &pb.ConfigurationSnapshot{
		Plugins: []*pb.PluginConfig{
			{Id: 20},
			{Id: 21, AuthPluginIds: []uint32{99}, LlmIds: []uint32{3}},
			{Id: 22, AuthPluginIds: []uint32{21}},
		},
		Llms: []*pb.LLMConfig{
			{Id: 1, Slug: "ok", PluginIds: []uint32{20}},
			{Id: 2, Slug: "runs-21", PluginIds: []uint32{21}},
			{Id: 3, Slug: "listed-on-21"},
		},
		Tools:           []*pb.ToolConfig{{Id: 4, Slug: "t-ok", AuthPluginIds: []uint32{20}}, {Id: 5, Slug: "t-gone", AuthPluginIds: []uint32{98}}},
		Datasources:     []*pb.DatasourceConfig{{Id: 6, AuthPluginIds: []uint32{98}}, {Id: 7}},
		ModelRouters:    []*pb.ModelRouterConfig{{Id: 8, Slug: "mr", AuthPluginIds: []uint32{98}}},
		SemanticRouters: []*pb.SemanticRouterConfig{{Id: 9, Slug: "sr", AuthPluginIds: []uint32{98}}},
		Apps:            []*pb.AppConfig{{Id: 30, LlmIds: []uint32{1, 2, 3}, ToolIds: []uint32{4, 5}, DatasourceIds: []uint32{6, 7}, ModelRouterIds: []uint32{8}, SemanticRouterIds: []uint32{9}}},
	}
	out, left := Sanitize(in)
	assert.Equal(t, []string{
		"LLM 2 left out: its plugin 21 is not in the configuration, and it does not run without it",
		"LLM 3 left out: its plugin 21 is not in the configuration, and it does not run without it",
		"tool 5 left out: its auth plugin 98 is not in the configuration, and it does not run without it",
		"model router 8 left out: its auth plugin 98 is not in the configuration, and it does not run without it",
		"semantic router 9 left out: its auth plugin 98 is not in the configuration, and it does not run without it",
		"datasource 6 left out: its auth plugin 98 is not in the configuration, and it does not run without it",
		"plugin 21 left out: its auth plugin 99 is not in the configuration, and it does not run without it",
		"plugin 22 left out: its auth plugin 21 is not in the configuration, and it does not run without it",
	}, Strings(left))
	require.Len(t, out.Plugins, 1)
	assert.Equal(t, uint32(20), out.Plugins[0].Id)
	require.Len(t, out.Llms, 1)
	assert.Equal(t, uint32(1), out.Llms[0].Id)
	app := out.Apps[0]
	assert.Equal(t, []uint32{1}, app.LlmIds)
	assert.Equal(t, []uint32{4}, app.ToolIds)
	assert.Equal(t, []uint32{7}, app.DatasourceIds)
	assert.Empty(t, app.ModelRouterIds)
	assert.Empty(t, app.SemanticRouterIds)
}

func TestSanitize_Nil(t *testing.T) {
	out, left := Sanitize(nil)
	assert.Nil(t, out)
	assert.Nil(t, left)
}
