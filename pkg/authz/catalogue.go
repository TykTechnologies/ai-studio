package authz

import (
	"fmt"
	"sort"
	"sync"
)

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
}

// Groups is the display order of catalogue groups, aligned with the admin
// navigation so an administrator recognises the matrix layout.
var Groups = []string{
	"Analytics",
	"Plugins",
	"LLM management",
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
	mu        sync.RWMutex
	registry  = map[string]Resource{}
	groupRank = func() map[string]int {
		m := make(map[string]int, len(Groups))
		for i, g := range Groups {
			m[g] = i
		}
		return m
	}()
)

// Register adds a resource to the catalogue. It panics on a duplicate key,
// an unknown group, a missing label, or an invalid action, so mistakes
// surface at boot (and in every test that imports the package).
func Register(r Resource) {
	if r.Key == "" || r.Label == "" {
		panic(fmt.Sprintf("authz: resource %q needs a key and a label", r.Key))
	}
	if _, ok := groupRank[r.Group]; !ok {
		panic(fmt.Sprintf("authz: resource %q has unknown group %q", r.Key, r.Group))
	}
	if len(r.Actions) == 0 {
		panic(fmt.Sprintf("authz: resource %q has no actions", r.Key))
	}
	seen := map[Action]bool{}
	for _, a := range r.Actions {
		if !a.Valid() || seen[a] {
			panic(fmt.Sprintf("authz: resource %q has invalid or duplicate action %q", r.Key, a))
		}
		seen[a] = true
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[r.Key]; dup {
		panic(fmt.Sprintf("authz: resource %q registered twice", r.Key))
	}
	registry[r.Key] = r
}

// Catalogue returns a copy of every resource, sorted by group order then
// label. This is the payload of GET /api/v1/rbac/permissions.
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
		gi, gj := groupRank[out[i].Group], groupRank[out[j].Group]
		if gi != gj {
			return gi < gj
		}
		return out[i].Label < out[j].Label
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
)

// The built-in catalogue. Order here does not matter; Catalogue() sorts.
// Route blocks each resource governs are documented in features/RBAC.md.
func init() {
	// Analytics
	Register(Resource{Key: "analytics", Label: "Analytics", Group: "Analytics", Actions: readOnly,
		Description: "Usage, cost and token dashboards."})
	Register(Resource{Key: "proxy-logs", Label: "Proxy logs", Group: "Analytics", Actions: readOnly, Sensitive: true,
		Description: "Raw gateway request and response logs, including prompt and completion bodies."})

	// Plugins
	Register(Resource{Key: "plugins", Label: "Installed plugins", Group: "Plugins", Actions: crudx, Privileged: true,
		Description: "Install, configure, approve scopes for, and call plugins. Plugins run code with the scopes they are granted."})
	Register(Resource{Key: "marketplace", Label: "Marketplace", Group: "Plugins", Actions: crudx,
		Description: "Browse the marketplace and manage marketplace sources."})

	// LLM management
	Register(Resource{Key: "llms", Label: "LLM providers", Group: "LLM management", Actions: crud})
	Register(Resource{Key: "model-prices", Label: "Model prices", Group: "LLM management", Actions: crud})
	Register(Resource{Key: "model-routers", Label: "Model routers", Group: "LLM management", Actions: crud})

	// Context management
	Register(Resource{Key: "datasources", Label: "Data sources", Group: "Context management", Actions: crudx})
	Register(Resource{Key: "tools", Label: "Tools", Group: "Context management", Actions: crudx})
	Register(Resource{Key: "filters", Label: "Filters", Group: "Context management", Actions: crudx})
	Register(Resource{Key: "filestores", Label: "File stores", Group: "Context management", Actions: crud})
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
	Register(Resource{Key: "sso-profiles", Label: "Identity providers", Group: "Access", Actions: crud, Sensitive: true, Privileged: true,
		Description: "Single sign-on profiles, including identity provider secrets and group mappings."})

	// Governance
	Register(Resource{Key: "audit", Label: "Audit trail", Group: "Governance", Actions: readOnly, Sensitive: true})
	Register(Resource{Key: "compliance", Label: "Compliance", Group: "Governance", Actions: readOnly})
	Register(Resource{Key: "metadata", Label: "Metadata schemas", Group: "Governance", Actions: crud,
		Description: "Governed metadata schemas and vocabularies. Metadata on an object is governed by that object's permission."})
	Register(Resource{Key: "exports", Label: "Log exports", Group: "Governance", Sensitive: true,
		Actions:     []Action{ActionRead, ActionWrite},
		Description: "Bulk proxy log export jobs."})
	Register(Resource{Key: "webhooks", Label: "Webhooks", Group: "Governance", Actions: crudx, Sensitive: true, Privileged: true,
		Description: "Outbound webhook targets and the delivery log. Execute approves, revokes, tests and replays: it sends platform data to an external URL."})

	// Settings
	Register(Resource{Key: "secrets", Label: "Secrets", Group: "Settings", Actions: crud,
		Description: "Secret references. Values are never returned by the API."})
	Register(Resource{Key: "branding", Label: "Branding", Group: "Settings", Actions: []Action{ActionRead, ActionWrite}})

	// AI Portal
	Register(Resource{Key: "apps", Label: "Apps", Group: "AI Portal", Actions: crud})
	Register(Resource{Key: "credentials", Label: "Credentials", Group: "AI Portal", Actions: crud, Sensitive: true,
		Description: "App credentials. Reading a credential reveals its secret."})
	Register(Resource{Key: "edges", Label: "Edge gateways", Group: "AI Portal", Actions: crudx,
		Description: "Edge gateway instances, namespaces and configuration sync."})

	// Chat
	Register(Resource{Key: "chats", Label: "Chats", Group: "Chat", Actions: crud})
	Register(Resource{Key: "agents", Label: "Agents", Group: "Chat", Actions: crudx})
	Register(Resource{Key: "llm-settings", Label: "Model call settings", Group: "Chat", Actions: crud})
	Register(Resource{Key: "chat-history", Label: "Chat history", Group: "Chat", Actions: crud, Sensitive: true,
		Description: "Conversation transcripts."})

	// Catalogs
	Register(Resource{Key: "catalogues", Label: "LLM catalogues", Group: "Catalogs", Actions: crud})
	Register(Resource{Key: "data-catalogues", Label: "Data catalogues", Group: "Catalogs", Actions: crud})
	Register(Resource{Key: "tool-catalogues", Label: "Tool catalogues", Group: "Catalogs", Actions: crud})
}
