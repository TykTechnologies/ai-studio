// Package snapshotguard keeps one entry of a configuration snapshot that an
// edge cannot store from blocking the rest of it.
//
// Edges keep some snapshot keys under unique indexes: LLM and tool slugs,
// model price (model, vendor) pairs, router slugs per namespace, OAuth client
// IDs and access token hashes. A snapshot that repeats one of those keys
// cannot be applied as it is: the insert fails, the sync rolls back, and the
// edge keeps its previous configuration, every other change included.
//
// Sanitize leaves out each entry that repeats a key, keeping the one with the
// lowest ID so the hub and every edge keep the same one, removes the
// references other entries hold to what it left out, and reports each one.
// It never leaves out a filter, and never changes the filters or plugins of
// an LLM it keeps. An LLM or tool attached to a filter or plugin the
// snapshot does not carry is left out too, so it never runs without it, and
// so is an endpoint (tool, datasource, router, plugin) whose auth plugin is
// missing, so it never falls back to App keys; a plugin left out for that
// takes the LLMs that run it along. What it leaves out loses access (an
// App's grant to a left-out LLM or tool goes with it), it never gains any.
package snapshotguard

import (
	"fmt"
	"sort"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/protobuf/proto"
)

// Kinds of entries Sanitize can leave out, in the order it reports them.
const (
	KindLLM            = "LLM"
	KindTool           = "tool"
	KindModelPrice     = "model price"
	KindModelRouter    = "model router"
	KindSemanticRouter = "semantic router"
	KindOAuthClient    = "OAuth client"
	KindAccessToken    = "access token"
	KindDatasource     = "datasource"
	KindPlugin         = "plugin"
)

var kindOrder = map[string]int{
	KindLLM: 0, KindTool: 1, KindModelPrice: 2, KindModelRouter: 3,
	KindSemanticRouter: 4, KindOAuthClient: 5, KindAccessToken: 6,
	KindDatasource: 7, KindPlugin: 8,
}

// Entry is one entry Sanitize left out.
type Entry struct {
	Kind string
	ID   uint32
	// Key is the repeated value, as the edge stores it ("" for access
	// tokens, whose key is a token hash and is not reported).
	Key string
	// KeptID is the entry of the same kind that keeps the key.
	KeptID uint32
	// Missing, when set, is the filter or plugin the entry is attached to
	// that the snapshot does not carry ("filter 7"): the entry is left out
	// for that instead of for its key.
	Missing string
}

func (e Entry) String() string {
	switch {
	case e.Missing != "":
		return fmt.Sprintf("%s %d left out: its %s is not in the configuration, and it does not run without it", e.Kind, e.ID, e.Missing)
	case e.Kind == KindAccessToken:
		return fmt.Sprintf("%s %d left out: its token hash is already used by %s %d", e.Kind, e.ID, e.Kind, e.KeptID)
	case e.KeptID == e.ID:
		return fmt.Sprintf("%s %d left out: it appears more than once", e.Kind, e.ID)
	default:
		return fmt.Sprintf("%s %d left out: %s %q is already used by %s %d", e.Kind, e.ID, keyName(e.Kind), e.Key, e.Kind, e.KeptID)
	}
}

func keyName(kind string) string {
	switch kind {
	case KindModelPrice:
		return "model and vendor"
	case KindOAuthClient:
		return "client ID"
	default:
		return "slug"
	}
}

// Strings returns the entries one line each, for logs and status reports.
func Strings(entries []Entry) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.String()
	}
	return out
}

// keyed is one entry's identity for de-duplication.
type keyed struct {
	id  uint32
	key string
}

// repeats returns the indexes of the entries whose key is already taken,
// keeping per key the entry with the lowest ID, and reports them in out.
func repeats(kind string, items []keyed, hideKey bool, out *[]Entry) map[int]bool {
	if len(items) < 2 {
		return nil
	}
	kept := make(map[string]int, len(items)) // key -> index of the entry keeping it
	for i, it := range items {
		if j, ok := kept[it.key]; !ok || it.id < items[j].id {
			kept[it.key] = i
		}
	}
	var drop map[int]bool
	for i, it := range items {
		j := kept[it.key]
		if j == i {
			continue
		}
		if drop == nil {
			drop = map[int]bool{}
		}
		drop[i] = true
		key := it.key
		if hideKey {
			key = ""
		}
		*out = append(*out, Entry{Kind: kind, ID: it.id, Key: key, KeptID: items[j].id})
	}
	return drop
}

// Sanitize returns the snapshot an edge can store and the entries it left
// out. When nothing has to be left out it returns in itself; otherwise a
// copy, and in is not changed.
func Sanitize(in *pb.ConfigurationSnapshot) (*pb.ConfigurationSnapshot, []Entry) {
	if in == nil {
		return nil, nil
	}
	var left []Entry

	llms := make([]keyed, len(in.Llms))
	for i, l := range in.Llms {
		llms[i] = keyed{l.Id, l.Slug}
	}
	dropLLMs := repeats(KindLLM, llms, false, &left)

	tools := make([]keyed, len(in.Tools))
	for i, t := range in.Tools {
		tools[i] = keyed{t.Id, t.Slug}
	}
	dropTools := repeats(KindTool, tools, false, &left)

	prices := make([]keyed, len(in.ModelPrices))
	for i, p := range in.ModelPrices {
		prices[i] = keyed{p.Id, p.ModelName + "\x00" + p.Vendor}
	}
	dropPrices := repeats(KindModelPrice, prices, false, &left)

	routers := make([]keyed, len(in.ModelRouters))
	for i, r := range in.ModelRouters {
		routers[i] = keyed{r.Id, r.Slug + "\x00" + r.Namespace}
	}
	dropRouters := repeats(KindModelRouter, routers, false, &left)

	semantic := make([]keyed, len(in.SemanticRouters))
	for i, r := range in.SemanticRouters {
		semantic[i] = keyed{r.Id, r.Slug + "\x00" + r.Namespace}
	}
	dropSemantic := repeats(KindSemanticRouter, semantic, false, &left)

	clients := make([]keyed, len(in.OauthClients))
	for i, c := range in.OauthClients {
		clients[i] = keyed{c.Id, c.ClientId}
	}
	dropClients := repeats(KindOAuthClient, clients, false, &left)

	tokens := make([]keyed, len(in.AccessTokens))
	for i, t := range in.AccessTokens {
		tokens[i] = keyed{t.Id, t.TokenHash}
	}
	dropTokens := repeats(KindAccessToken, tokens, true, &left)

	// Attachments. An LLM or tool never runs without a filter or plugin
	// attached to it, and an endpoint never runs without its auth plugins
	// (it would fall back to App keys): one whose filter or plugin the
	// snapshot does not carry is left out. Filters are never left out. A
	// plugin is left out only when its own endpoint's auth plugin is
	// missing, and then whatever runs it goes too.
	filters := make(map[uint32]bool, len(in.Filters))
	for _, f := range in.Filters {
		filters[f.Id] = true
	}
	var dropPlugins map[int]bool
	plugins := map[uint32]bool{}
	for {
		for id := range plugins {
			delete(plugins, id)
		}
		for i, pl := range in.Plugins {
			if !dropPlugins[i] {
				plugins[pl.Id] = true
			}
		}
		changed := false
		for i, pl := range in.Plugins {
			if dropPlugins[i] {
				continue
			}
			if missing := missingAuth(pl.AuthPluginIds, plugins); missing != "" {
				dropPlugins = mark(dropPlugins, i)
				changed = true
				left = appendOnce(left, Entry{Kind: KindPlugin, ID: pl.Id, Missing: missing})
			}
		}
		if !changed {
			break
		}
	}
	// LLMs a left-out plugin was attached to (hubs before plugin_ids list
	// the attachment on the plugin only).
	runsGone := map[uint32]string{}
	for i, pl := range in.Plugins {
		if dropPlugins[i] {
			for _, llmID := range pl.LlmIds {
				runsGone[llmID] = fmt.Sprintf("plugin %d", pl.Id)
			}
		}
	}
	for i, l := range in.Llms {
		if dropLLMs[i] {
			continue
		}
		missing := missingGuard(l.FilterIds, l.PluginIds, filters, plugins)
		if missing == "" {
			missing = runsGone[l.Id]
		}
		if missing != "" {
			dropLLMs = mark(dropLLMs, i)
			left = append(left, Entry{Kind: KindLLM, ID: l.Id, Key: l.Slug, Missing: missing})
		}
	}
	for i, t := range in.Tools {
		if dropTools[i] {
			continue
		}
		missing := missingGuard(t.FilterIds, nil, filters, plugins)
		if missing == "" {
			missing = missingAuth(t.AuthPluginIds, plugins)
		}
		if missing != "" {
			dropTools = mark(dropTools, i)
			left = append(left, Entry{Kind: KindTool, ID: t.Id, Key: t.Slug, Missing: missing})
		}
	}
	var dropDatasources map[int]bool
	for i, d := range in.Datasources {
		if missing := missingAuth(d.AuthPluginIds, plugins); missing != "" {
			dropDatasources = mark(dropDatasources, i)
			left = append(left, Entry{Kind: KindDatasource, ID: d.Id, Missing: missing})
		}
	}
	for i, r := range in.ModelRouters {
		if missing := missingAuth(r.AuthPluginIds, plugins); missing != "" && !dropRouters[i] {
			dropRouters = mark(dropRouters, i)
			left = append(left, Entry{Kind: KindModelRouter, ID: r.Id, Key: r.Slug, Missing: missing})
		}
	}
	for i, r := range in.SemanticRouters {
		if missing := missingAuth(r.AuthPluginIds, plugins); missing != "" && !dropSemantic[i] {
			dropSemantic = mark(dropSemantic, i)
			left = append(left, Entry{Kind: KindSemanticRouter, ID: r.Id, Key: r.Slug, Missing: missing})
		}
	}

	if len(left) == 0 {
		return in, nil
	}
	for i := range left {
		// The composite keys are reported in their readable form.
		if left[i].Missing == "" {
			left[i].Key = readableKey(left[i])
		}
	}
	sort.SliceStable(left, func(i, j int) bool {
		if left[i].Kind != left[j].Kind {
			return kindOrder[left[i].Kind] < kindOrder[left[j].Kind]
		}
		return left[i].ID < left[j].ID
	})

	out := proto.Clone(in).(*pb.ConfigurationSnapshot)
	out.Llms = keep(out.Llms, dropLLMs)
	out.Tools = keep(out.Tools, dropTools)
	out.ModelPrices = keep(out.ModelPrices, dropPrices)
	out.ModelRouters = keep(out.ModelRouters, dropRouters)
	out.SemanticRouters = keep(out.SemanticRouters, dropSemantic)
	out.OauthClients = keep(out.OauthClients, dropClients)
	out.AccessTokens = keep(out.AccessTokens, dropTokens)
	out.Datasources = keep(out.Datasources, dropDatasources)
	out.Plugins = keep(out.Plugins, dropPlugins)

	goneLLMs := idsOf(in.Llms, dropLLMs, func(l *pb.LLMConfig) uint32 { return l.Id })
	goneTools := idsOf(in.Tools, dropTools, func(t *pb.ToolConfig) uint32 { return t.Id })
	goneRouters := idsOf(in.ModelRouters, dropRouters, func(r *pb.ModelRouterConfig) uint32 { return r.Id })
	goneSemantic := idsOf(in.SemanticRouters, dropSemantic, func(r *pb.SemanticRouterConfig) uint32 { return r.Id })
	goneDatasources := idsOf(in.Datasources, dropDatasources, func(d *pb.DatasourceConfig) uint32 { return d.Id })

	// A left-out entry's ID may also be the ID of an entry that is kept
	// (the same row sent twice): its references then stay.
	keptIDs := func(items []keyed, drop map[int]bool) map[uint32]bool {
		ids := map[uint32]bool{}
		for i, it := range items {
			if !drop[i] {
				ids[it.id] = true
			}
		}
		return ids
	}
	for id := range keptIDs(llms, dropLLMs) {
		delete(goneLLMs, id)
	}
	for id := range keptIDs(tools, dropTools) {
		delete(goneTools, id)
	}
	for id := range keptIDs(routers, dropRouters) {
		delete(goneRouters, id)
	}
	for id := range keptIDs(semantic, dropSemantic) {
		delete(goneSemantic, id)
	}

	// Grants to what was left out go with it: an App never reaches a
	// left-out LLM or tool through the entry that kept its slug.
	for _, app := range out.Apps {
		app.LlmIds = without(app.LlmIds, goneLLMs)
		app.ToolIds = without(app.ToolIds, goneTools)
		app.ModelRouterIds = without(app.ModelRouterIds, goneRouters)
		app.SemanticRouterIds = without(app.SemanticRouterIds, goneSemantic)
		app.DatasourceIds = without(app.DatasourceIds, goneDatasources)
	}
	for _, f := range out.Filters {
		f.LlmIds = without(f.LlmIds, goneLLMs)
	}
	for _, p := range out.Plugins {
		p.LlmIds = without(p.LlmIds, goneLLMs)
	}
	// A model router vendor names its LLM by slug as well as by ID, and the
	// edge routes by the slug: a vendor of a left-out LLM would reach the
	// LLM that kept the slug. It is removed instead.
	if len(goneLLMs) > 0 {
		for _, r := range out.ModelRouters {
			for _, pool := range r.Pools {
				vendors := pool.Vendors[:0]
				for _, v := range pool.Vendors {
					if !goneLLMs[v.LlmId] {
						vendors = append(vendors, v)
					}
				}
				pool.Vendors = vendors
			}
		}
	}
	return out, left
}

func mark(drop map[int]bool, i int) map[int]bool {
	if drop == nil {
		drop = map[int]bool{}
	}
	drop[i] = true
	return drop
}

// missingGuard names the first of the filters and plugins that is not in
// the snapshot, or returns "".
func missingGuard(filterIDs, pluginIDs []uint32, filters, plugins map[uint32]bool) string {
	for _, id := range filterIDs {
		if !filters[id] {
			return fmt.Sprintf("filter %d", id)
		}
	}
	for _, id := range pluginIDs {
		if !plugins[id] {
			return fmt.Sprintf("plugin %d", id)
		}
	}
	return ""
}

// missingAuth names the first auth plugin that is not in the snapshot, or
// returns "".
func missingAuth(pluginIDs []uint32, plugins map[uint32]bool) string {
	for _, id := range pluginIDs {
		if !plugins[id] {
			return fmt.Sprintf("auth plugin %d", id)
		}
	}
	return ""
}

// appendOnce adds e unless an entry of the same kind and ID is reported
// already (a plugin row a hub sent more than once).
func appendOnce(left []Entry, e Entry) []Entry {
	for _, l := range left {
		if l.Kind == e.Kind && l.ID == e.ID {
			return left
		}
	}
	return append(left, e)
}

func readableKey(e Entry) string {
	switch e.Kind {
	case KindModelPrice, KindModelRouter, KindSemanticRouter:
		for i := 0; i < len(e.Key); i++ {
			if e.Key[i] == 0 {
				first, second := e.Key[:i], e.Key[i+1:]
				if e.Kind == KindModelPrice {
					return first + " / " + second
				}
				if second == "" {
					return first
				}
				return first + " (namespace " + second + ")"
			}
		}
	}
	return e.Key
}

func keep[T any](items []T, drop map[int]bool) []T {
	if len(drop) == 0 {
		return items
	}
	out := make([]T, 0, len(items)-len(drop))
	for i, it := range items {
		if !drop[i] {
			out = append(out, it)
		}
	}
	return out
}

func idsOf[T any](items []T, drop map[int]bool, id func(T) uint32) map[uint32]bool {
	ids := make(map[uint32]bool, len(drop))
	for i := range drop {
		ids[id(items[i])] = true
	}
	return ids
}

func without(ids []uint32, gone map[uint32]bool) []uint32 {
	if len(gone) == 0 || len(ids) == 0 {
		return ids
	}
	out := ids[:0]
	for _, id := range ids {
		if !gone[id] {
			out = append(out, id)
		}
	}
	return out
}
