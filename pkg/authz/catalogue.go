package authz

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ScopeKind says how Workspaces bound a resource's permissions. Objects of
// a non-global resource belong to a Workspace: their own ownership stamp
// (workspace_id), or for ScopeWorkspaceParent the stamp of their parent
// object. A binding scoped to a Workspace grants permissions on them only
// inside that Workspace. A scoped binding grants nothing on a global
// resource.
type ScopeKind string

const (
	// ScopeGlobal (the zero value) is platform configuration with no owning
	// Workspace: users, roles, edges, plugins, webhooks, ... Only a global
	// binding grants it.
	ScopeGlobal ScopeKind = ""
	// ScopeWorkspaceWrite resources are read-global and write-scoped: read in
	// any scope reads all of them; write, delete, execute and publish need a
	// binding that covers the object's Workspace.
	ScopeWorkspaceWrite ScopeKind = "workspace_write"
	// ScopeWorkspaceFull resources are read- and write-scoped: objects in a
	// Workspace the caller's read does not cover are hidden (404).
	ScopeWorkspaceFull ScopeKind = "workspace_full"
	// ScopeWorkspaceParent resources have no stamp of their own: they belong
	// to the Workspace of a parent object (Resource.ScopeParent, a
	// ScopeWorkspaceFull resource) and follow it when it moves. Read and
	// write are scoped like the parent's: a hidden parent hides them (404),
	// and every action needs the permission in the parent's Workspace.
	// App credentials and MCP credentials (parent: apps).
	ScopeWorkspaceParent ScopeKind = "workspace_parent"
)

// Valid reports whether k is one of the known kinds.
func (k ScopeKind) Valid() bool {
	switch k {
	case ScopeGlobal, ScopeWorkspaceWrite, ScopeWorkspaceFull, ScopeWorkspaceParent:
		return true
	}
	return false
}

// InWorkspace reports whether permissions on a resource of this kind can be
// granted inside a Workspace (by a Workspace-scoped binding).
func (k ScopeKind) InWorkspace() bool { return k != ScopeGlobal }

// Stamped reports whether objects of a resource of this kind carry their
// own ownership stamp (workspace_id).
func (k ScopeKind) Stamped() bool { return k == ScopeWorkspaceWrite || k == ScopeWorkspaceFull }

// ReadScoped reports whether reading an object of this kind needs read
// covering its Workspace (otherwise the object is hidden, 404).
func (k ScopeKind) ReadScoped() bool { return k == ScopeWorkspaceFull || k == ScopeWorkspaceParent }

// Resource is one row of the permission catalogue.
type Resource struct {
	// Key is the permission resource slug: plural, kebab-case, matching the
	// route collection segment ("llms", "data-catalogues").
	Key string `json:"key"`
	// Label is the human name, matching the admin navigation where one exists.
	Label string `json:"label"`
	// Group is the navigation group the resource belongs to; see Groups.
	Group string `json:"group"`
	// Sensitive marks a data class (transcripts, logs, secrets) that is split
	// out so read can be withheld independently. The UI shows a shield.
	Sensitive bool `json:"sensitive"`
	// Privileged marks resources whose write/delete can escalate access
	// (users, groups, roles, identity providers, plugins). The UI warns.
	Privileged bool `json:"privileged"`
	// Actions lists the actions this resource offers, in display order.
	Actions []Action `json:"actions"`
	// Description is optional help text for the role editor.
	Description string `json:"description,omitempty"`
	// Plugin is the permission key of the plugin that contributed this
	// resource ("plugin:<manifest id>"), or "" for built-in resources. The
	// role editor groups plugin resources under their plugin.
	Plugin string `json:"plugin,omitempty"`
	// PluginLabel is the display name of the contributing plugin.
	PluginLabel string `json:"plugin_label,omitempty"`
	// Dynamic marks a resource registered at runtime (plugins) rather than
	// in init(); dynamic resources can be replaced and unregistered.
	Dynamic bool `json:"dynamic,omitempty"`
	// Scope says whether the resource's objects belong to a Workspace and how
	// Workspace-scoped bindings apply to it; see ScopeKind. Plugin resources
	// are always global.
	Scope ScopeKind `json:"scope,omitempty"`
	// ScopeParent is, for ScopeWorkspaceParent, the key of the resource whose
	// objects own this one's ("apps" for credentials). Empty otherwise.
	ScopeParent string `json:"scope_parent,omitempty"`
}

// PluginResourcePrefix starts every plugin-contributed resource key. The
// umbrella rule in Set.Has grants all of them to holders of plugins:execute.
const PluginResourcePrefix = "plugin:"

// IsPluginResource reports whether key belongs to a plugin.
func IsPluginResource(key string) bool {
	return strings.HasPrefix(key, PluginResourcePrefix)
}

// Groups is the display order of catalogue groups, aligned with the admin
// navigation so an administrator recognises the matrix layout.
var Groups = []string{
	"Analytics",
	"Plugins",
	"Model management",
	"Context management",
	"Community",
	"Access",
	"Governance",
	"Settings",
	"AI Portal",
	"Chat",
	"Catalogs",
}

var (
	mu       sync.RWMutex
	registry = map[string]Resource{}
	version  uint64
	// scopedCache holds the keys of every non-global resource, sorted; nil
	// means "recompute". Register clears it. Plugin resources are always
	// global, so Replace and Unregister leave it alone.
	scopedCache []string
	groupRank   = func() map[string]int {
		m := make(map[string]int, len(Groups))
		for i, g := range Groups {
			m[g] = i
		}
		return m
	}()
)

func validate(r Resource) error {
	if r.Key == "" || r.Label == "" {
		return fmt.Errorf("authz: resource %q needs a key and a label", r.Key)
	}
	if _, ok := groupRank[r.Group]; !ok {
		return fmt.Errorf("authz: resource %q has unknown group %q", r.Key, r.Group)
	}
	if len(r.Actions) == 0 {
		return fmt.Errorf("authz: resource %q has no actions", r.Key)
	}
	if r.Actions[0] != ActionRead {
		return fmt.Errorf("authz: resource %q must offer read first", r.Key)
	}
	seen := map[Action]bool{}
	for _, a := range r.Actions {
		if !a.Valid() || seen[a] {
			return fmt.Errorf("authz: resource %q has invalid or duplicate action %q", r.Key, a)
		}
		seen[a] = true
	}
	if r.Dynamic != IsPluginResource(r.Key) {
		return fmt.Errorf("authz: resource %q: only plugin resources (%q prefix) may be dynamic", r.Key, PluginResourcePrefix)
	}
	if !r.Scope.Valid() {
		return fmt.Errorf("authz: resource %q has unknown scope kind %q", r.Key, r.Scope)
	}
	if (r.Scope == ScopeWorkspaceParent) != (r.ScopeParent != "") || (r.ScopeParent != "" && r.ScopeParent == r.Key) {
		return fmt.Errorf("authz: resource %q: a parent scope needs a parent resource, and only it may name one", r.Key)
	}
	if r.Dynamic && r.Scope != ScopeGlobal {
		return fmt.Errorf("authz: plugin resource %q must be global", r.Key)
	}
	if r.Dynamic && (r.Plugin == "" || !strings.HasPrefix(r.Key, r.Plugin)) {
		return fmt.Errorf("authz: plugin resource %q must name its plugin as the key prefix", r.Key)
	}
	return nil
}

// Register adds a built-in resource to the catalogue. It panics on a
// duplicate key, an unknown group, a missing label, or an invalid action, so
// mistakes surface at boot (and in every test that imports the package).
// Plugin resources use Replace instead.
func Register(r Resource) {
	if err := validate(r); err != nil {
		panic(err.Error())
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[r.Key]; dup {
		panic(fmt.Sprintf("authz: resource %q registered twice", r.Key))
	}
	registry[r.Key] = r
	scopedCache = nil
}

// Replace upserts a plugin resource. It returns an error instead of
// panicking because plugin manifests are user data, and it bumps Version so
// clients can refresh the catalogue.
func Replace(r Resource) error {
	r.Dynamic = true
	if err := validate(r); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if existing, ok := registry[r.Key]; ok && !existing.Dynamic {
		return fmt.Errorf("authz: resource %q is built in and cannot be replaced", r.Key)
	}
	registry[r.Key] = r
	version++
	return nil
}

// Unregister removes a plugin resource. Built-in resources are never
// removed. It reports whether anything changed.
func Unregister(key string) bool {
	mu.Lock()
	defer mu.Unlock()
	r, ok := registry[key]
	if !ok || !r.Dynamic {
		return false
	}
	delete(registry, key)
	version++
	return true
}

// UnregisterPlugin removes every resource the plugin contributed (its base
// resource and any sub-resources) and reports how many were removed.
func UnregisterPlugin(plugin string) int {
	if plugin == "" {
		return 0
	}
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for key, r := range registry {
		if r.Dynamic && r.Plugin == plugin {
			delete(registry, key)
			n++
		}
	}
	if n > 0 {
		version++
	}
	return n
}

// PluginResources returns the resources contributed by one plugin, sorted
// with the base resource (key == plugin) first.
func PluginResources(plugin string) []Resource {
	var out []Resource
	for _, r := range Catalogue() {
		if r.Plugin == plugin {
			out = append(out, r)
		}
	}
	return out
}

// Version counts dynamic catalogue changes since boot. It is sent with
// GET /api/v1/rbac/permissions so the UI can tell a stale copy apart.
func Version() uint64 {
	mu.RLock()
	defer mu.RUnlock()
	return version
}

// Catalogue returns a copy of every resource, sorted by group order, then
// built-in resources by label, then plugin resources by plugin label with
// each plugin's base resource ahead of its sub-resources. This is the
// payload of GET /api/v1/rbac/permissions.
func Catalogue() []Resource {
	mu.RLock()
	out := make([]Resource, 0, len(registry))
	for _, r := range registry {
		cp := r
		cp.Actions = append([]Action(nil), r.Actions...)
		out = append(out, cp)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ga, gb := groupRank[a.Group], groupRank[b.Group]; ga != gb {
			return ga < gb
		}
		if a.Dynamic != b.Dynamic {
			return !a.Dynamic
		}
		if a.Plugin != b.Plugin {
			if a.PluginLabel != b.PluginLabel {
				return a.PluginLabel < b.PluginLabel
			}
			return a.Plugin < b.Plugin
		}
		if (a.Key == a.Plugin) != (b.Key == b.Plugin) {
			return a.Key == a.Plugin
		}
		return a.Label < b.Label
	})
	return out
}

// Lookup returns the resource for a concrete permission, and whether the
// resource offers that action.
func Lookup(p Permission) (Resource, bool) {
	res := p.Resource()
	act := p.Action()
	if res == "" || act == "" {
		return Resource{}, false
	}
	mu.RLock()
	r, ok := registry[res]
	mu.RUnlock()
	if !ok {
		return Resource{}, false
	}
	for _, a := range r.Actions {
		if a == act {
			return r, true
		}
	}
	return Resource{}, false
}

// ResourceByKey returns the resource with the given key.
func ResourceByKey(key string) (Resource, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := registry[key]
	return r, ok
}

// ScopeOf returns the scope kind of a permission's resource. Sentinels,
// unknown resources and plugin resources are ScopeGlobal, so anything the
// catalogue does not know is never granted by a Workspace-scoped binding.
func ScopeOf(p Permission) ScopeKind {
	return ResourceScope(p.Resource())
}

// ResourceScope returns the scope kind of a resource key; ScopeGlobal for
// an unknown key.
func ResourceScope(key string) ScopeKind {
	if key == "" {
		return ScopeGlobal
	}
	mu.RLock()
	r, ok := registry[key]
	mu.RUnlock()
	if !ok {
		return ScopeGlobal
	}
	return r.Scope
}

// WorkspaceResources returns, sorted, the keys of every resource whose
// permissions a Workspace-scoped binding can grant (every non-global
// kind). Only built-in resources can be non-global, so the list is computed
// once and recomputed only after a Register.
func WorkspaceResources() []string {
	mu.RLock()
	cached := scopedCache
	mu.RUnlock()
	if cached == nil {
		mu.Lock()
		if scopedCache == nil {
			scopedCache = []string{}
			for k, r := range registry {
				if r.Scope.InWorkspace() {
					scopedCache = append(scopedCache, k)
				}
			}
			sort.Strings(scopedCache)
		}
		cached = scopedCache
		mu.Unlock()
	}
	return append([]string(nil), cached...)
}

// StampedResources returns, sorted, the keys of every resource whose
// objects carry their own workspace_id stamp.
func StampedResources() []string {
	var out []string
	for _, key := range WorkspaceResources() {
		if ResourceScope(key).Stamped() {
			out = append(out, key)
		}
	}
	return out
}

// All returns every concrete permission in the catalogue, sorted.
func All() []Permission {
	var out []Permission
	for _, r := range Catalogue() {
		for _, a := range r.Actions {
			out = append(out, P(r.Key, a))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AllWithAction returns every concrete permission carrying the action.
func AllWithAction(a Action) []Permission {
	var out []Permission
	for _, p := range All() {
		if p.Action() == a {
			out = append(out, p)
		}
	}
	return out
}

// Filter returns the permissions from All() the predicate keeps.
func Filter(keep func(Resource, Action) bool) []Permission {
	var out []Permission
	for _, r := range Catalogue() {
		for _, a := range r.Actions {
			if keep(r, a) {
				out = append(out, P(r.Key, a))
			}
		}
	}
	return out
}

var (
	crud     = []Action{ActionRead, ActionWrite, ActionDelete}
	crudx    = []Action{ActionRead, ActionWrite, ActionDelete, ActionExecute}
	readOnly = []Action{ActionRead}
	// Resources with a live switch (active/enabled) additionally offer
	// publish; see ActionPublish.
	crudp  = []Action{ActionRead, ActionWrite, ActionDelete, ActionPublish}
	crudxp = []Action{ActionRead, ActionWrite, ActionDelete, ActionExecute, ActionPublish}
)

// Publishable returns the keys of every resource offering ActionPublish.
func Publishable() []string {
	var out []string
	for _, r := range Catalogue() {
		for _, a := range r.Actions {
			if a == ActionPublish {
				out = append(out, r.Key)
				break
			}
		}
	}
	return out
}

// The built-in catalogue. Order here does not matter; Catalogue() sorts.
// Route blocks each resource governs are documented in features/RBAC.md.
func init() {
	// Analytics
	Register(Resource{Key: "analytics", Label: "Analytics", Group: "Analytics", Actions: readOnly,
		Description: "Usage, cost and token dashboards."})
	Register(Resource{Key: "proxy-logs", Label: "Proxy logs", Group: "Analytics", Actions: readOnly, Sensitive: true,
		Description: "Raw gateway request and response logs, including prompt and completion bodies."})
	// Agent Call Tracing (Enterprise). Metadata and content are split so
	// read can be withheld on the content alone (spec §9.1).
	Register(Resource{Key: "traces", Label: "Agent traces", Group: "Analytics", Actions: crudx,
		Description: "Agent sessions, traces and spans: timings, tokens, cost, tool names and gateway events. End users and external ids appear as pseudonyms. Write changes tracing settings, delete purges sessions, execute exports."})
	Register(Resource{Key: "trace-content", Label: "Agent trace content", Group: "Analytics", Actions: readOnly, Sensitive: true,
		Description: "Raw end-user and external ids, plus captured prompts, responses and tool data in agent traces (subject to each App's content mode). Every view is audited."})

	// Plugins. Global although plugin rows carry a workspace_id stamp: an
	// enabled plugin acts on every Workspace's objects, so a Workspace cannot
	// contain it.
	Register(Resource{Key: "plugins", Label: "Installed plugins", Group: "Plugins", Actions: crudxp, Privileged: true,
		Description: "Install, configure, approve scopes for, and call plugins. Publish enables or disables an installed plugin. Plugins run code with the scopes they are granted."})
	Register(Resource{Key: "marketplace", Label: "Marketplace", Group: "Plugins", Actions: crudx,
		Description: "Browse the marketplace and manage marketplace sources."})

	// LLM management
	Register(Resource{Key: "llms", Scope: ScopeWorkspaceWrite, Label: "Model providers", Group: "Model management", Actions: crudp,
		Description: "Publish sets a provider active; only active providers are served by the gateway and offered in the portal."})
	Register(Resource{Key: "model-prices", Label: "Model prices", Group: "Model management", Actions: crud})
	Register(Resource{Key: "model-routers", Scope: ScopeWorkspaceFull, Label: "Model routers", Group: "Model management", Actions: crudp,
		Description: "Publish sets a router active; only active routers are served."})
	Register(Resource{Key: "semantic-routers", Scope: ScopeWorkspaceFull, Label: "Semantic routers", Group: "Model management", Actions: crudp,
		Description: "Routers that pick a model from what the prompt says. Publish sets a router active; only active routers are served."})
	Register(Resource{Key: "embedders", Scope: ScopeWorkspaceWrite, Label: "Embedders", Group: "Model management", Actions: crud,
		Description: "Reusable embedding configurations that data sources and semantic routers embed with. Creating one inline from those forms also needs this permission."})

	// Context management
	Register(Resource{Key: "datasources", Scope: ScopeWorkspaceWrite, Label: "Data sources", Group: "Context management", Actions: crudxp,
		Description: "Publish sets a data source active; only active data sources are served."})
	Register(Resource{Key: "tools", Scope: ScopeWorkspaceWrite, Label: "Tools", Group: "Context management", Actions: crudxp,
		Description: "Publish sets a tool active; only active tools are served."})
	Register(Resource{Key: "mcp-servers", Scope: ScopeWorkspaceWrite, Label: "MCP servers", Group: "Context management", Actions: crudxp,
		Description: "MCP servers proxied by a Tyk Gateway. Execute registers, pushes and links definitions on the Tyk Dashboard and creates policies; publish makes a server visible in the portal."})
	Register(Resource{Key: "filters", Scope: ScopeWorkspaceWrite, Label: "Filters", Group: "Context management", Actions: crudx})
	Register(Resource{Key: "filestores", Scope: ScopeWorkspaceWrite, Label: "File stores", Group: "Context management", Actions: crud})
	Register(Resource{Key: "tags", Label: "Tags", Group: "Context management", Actions: crud})

	// Community
	Register(Resource{Key: "submissions", Label: "Submission queue", Group: "Community",
		Actions:     []Action{ActionRead, ActionWrite, ActionExecute},
		Description: "Review, approve and reject community submissions."})
	Register(Resource{Key: "attestation-templates", Label: "Attestation templates", Group: "Community", Actions: crud})

	// Access
	Register(Resource{Key: "users", Label: "Users", Group: "Access", Actions: crud, Privileged: true,
		Description: "Manage user accounts and roll API keys."})
	Register(Resource{Key: "groups", Label: "Teams", Group: "Access", Actions: crud, Privileged: true,
		Description: "Manage teams and their membership. Roles bound to a team apply to all of its members."})
	Register(Resource{Key: "roles", Label: "Roles", Group: "Access", Actions: crud, Privileged: true,
		Description: "Define roles and assign them to users and teams."})
	Register(Resource{Key: "workspaces", Label: "Workspaces", Group: "Access", Actions: crud, Privileged: true,
		Description: "Create, rename and delete Workspaces. Objects belong to a Workspace; a role assigned within a Workspace applies only to that Workspace's objects."})
	Register(Resource{Key: "sso-profiles", Label: "Identity providers", Group: "Access", Actions: crud, Sensitive: true, Privileged: true,
		Description: "Single sign-on profiles, including identity provider secrets and group mappings."})

	// Governance
	Register(Resource{Key: "audit", Label: "Audit trail", Group: "Governance", Actions: readOnly, Sensitive: true})
	Register(Resource{Key: "compliance", Label: "Compliance", Group: "Governance", Actions: readOnly})
	Register(Resource{Key: "metadata", Label: "Metadata schemas", Group: "Governance", Actions: crudp,
		Description: "Governed metadata schemas and vocabularies. Publish activates a schema. Metadata on an object is governed by that object's permission."})
	Register(Resource{Key: "exports", Label: "Log exports", Group: "Governance", Sensitive: true,
		Actions:     []Action{ActionRead, ActionWrite},
		Description: "Bulk proxy log export jobs."})
	// Global although webhook targets carry a workspace_id stamp: targets
	// receive system events of every Workspace.
	Register(Resource{Key: "webhooks", Label: "Webhooks", Group: "Governance", Actions: crudx, Sensitive: true, Privileged: true,
		Description: "Outbound webhook targets and the delivery log. Execute approves, revokes, tests and replays: it sends platform data to an external URL."})

	// Settings
	Register(Resource{Key: "tyk-connections", Label: "Tyk connections", Group: "Settings", Actions: crudx, Sensitive: true, Privileged: true,
		Description: "Connections to Tyk Dashboards for MCP proxy discovery, registration and key brokering. Execute activates, disables, probes and syncs a connection, and allows internal Dashboard hosts."})
	Register(Resource{Key: "secrets", Scope: ScopeWorkspaceWrite, Label: "Secrets", Group: "Settings", Actions: crud,
		Description: "Secret references. Values are never returned by the API."})
	Register(Resource{Key: "branding", Label: "Branding", Group: "Settings", Actions: []Action{ActionRead, ActionWrite}})

	// AI Portal
	Register(Resource{Key: "apps", Scope: ScopeWorkspaceFull, Label: "Apps", Group: "AI Portal", Actions: crudp,
		Description: "Publish sets an app active; inactive apps cannot authenticate against the gateway."})
	Register(Resource{Key: "credentials", Scope: ScopeWorkspaceParent, ScopeParent: "apps", Label: "Credentials", Group: "AI Portal", Actions: crud, Sensitive: true,
		Description: "App credentials. Reading a credential reveals its secret."})
	Register(Resource{Key: "mcp-credentials", Scope: ScopeWorkspaceParent, ScopeParent: "apps", Label: "MCP credentials", Group: "AI Portal", Actions: crudx, Sensitive: true, Privileged: true,
		Description: "Tyk access keys minted for Apps that use MCP servers. Execute mints, rotates, suspends, revokes and applies widening changes."})
	Register(Resource{Key: "edges", Label: "Edge gateways", Group: "AI Portal", Actions: crudx,
		Description: "Edge gateway instances, namespaces and configuration sync."})

	// Chat
	Register(Resource{Key: "chats", Label: "Chats", Group: "Chat", Actions: crud})
	Register(Resource{Key: "agents", Scope: ScopeWorkspaceFull, Label: "Agents", Group: "Chat", Actions: crudxp,
		Description: "Publish sets an agent active so it can be used in chat."})
	Register(Resource{Key: "llm-settings", Label: "Model call settings", Group: "Chat", Actions: crud})
	Register(Resource{Key: "chat-history", Label: "Chat history", Group: "Chat", Actions: crud, Sensitive: true,
		Description: "Conversation transcripts."})

	// Catalogs
	Register(Resource{Key: "catalogues", Scope: ScopeWorkspaceFull, Label: "LLM catalogues", Group: "Catalogs", Actions: crud})
	Register(Resource{Key: "data-catalogues", Scope: ScopeWorkspaceFull, Label: "Data catalogues", Group: "Catalogs", Actions: crud})
	Register(Resource{Key: "tool-catalogues", Scope: ScopeWorkspaceFull, Label: "Tool catalogues", Group: "Catalogs", Actions: crud})
}
