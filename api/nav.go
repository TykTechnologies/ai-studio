package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// NavItem is one entry of the console's navigation: a page (Path) or a
// group of pages (Items). Paths are the console's own routes, without the
// base path. Icon names a Font Awesome icon the console ships. Permission
// is the one that unlocks the entry; the manifest only lists entries the
// user may open, so a host need not check it.
type NavItem struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Path       string    `json:"path,omitempty"`
	Icon       string    `json:"icon,omitempty"`
	Exact      bool      `json:"exact,omitempty"`
	Title      string    `json:"title,omitempty"`
	Permission string    `json:"permission,omitempty"`
	PluginID   uint      `json:"pluginId,omitempty"`
	Items      []NavItem `json:"items,omitempty"`
}

// NavManifest is the navigation a user may see: the surfaces (Admin,
// Portal, Chat) the console's top bar switches between, and the admin
// drawer. The console's admin drawer renders from it, and a host that draws
// its own navigation (Options.Chromeless) builds its menu from it.
type NavManifest struct {
	Surfaces []NavItem `json:"surfaces"`
	Admin    []NavItem `json:"admin"`
}

// navInputs is what the admin menu depends on besides permissions.
type navInputs struct {
	features   func(string) bool
	enterprise bool
	// identityProviders shows the identity provider pages (SSO is available,
	// Studio signs users in itself, and the user may configure it).
	identityProviders bool
	// plugins are the plugin-contributed sections, already filtered by
	// permission.
	plugins []services.SidebarMenuItem
}

func when(cond bool, items ...NavItem) []NavItem {
	if cond {
		return items
	}
	return nil
}

func group(id, text, icon string, items ...[]NavItem) NavItem {
	g := NavItem{ID: id, Text: text, Icon: icon}
	for _, part := range items {
		g.Items = append(g.Items, part...)
	}
	return g
}

func page(id, text, path string, perm authz.Permission) NavItem {
	return NavItem{ID: id, Text: text, Path: path, Permission: string(perm)}
}

// adminNav is the admin menu before permission filtering. Group order: the
// pages an administrator visits daily first (who may use what: Access, then
// the Catalogs that grant it), the things being governed next, then the
// surfaces (Portal, Community, Governance), plugin sections in one
// predictable place, and system pages last.
func adminNav(in navInputs) []NavItem {
	f := in.features
	r := authz.Read
	out := []NavItem{
		{ID: "overview", Text: "Overview", Icon: "house", Path: "/admin", Exact: true},
		{ID: "dashboard", Text: "Analytics", Icon: "monitor-waveform", Path: "/admin/dash", Permission: string(r("analytics"))},
		group("access", "Access", "users",
			[]NavItem{page("users", "Users", "/admin/users", r("users"))},
			when(f("feature_groups") && (!f("feature_gateway") || f("feature_portal") || f("feature_chat")),
				page("groups", "Teams", "/admin/groups", r("groups"))),
			when(f("feature_rbac"), page("roles", "Roles", "/admin/roles", r("roles"))),
			when(in.identityProviders, page("sso-profiles", "Identity providers", "/admin/sso-profiles", r("sso-profiles"))),
		),
	}
	// Catalogs are how teams are granted providers, data sources and tools,
	// so they sit directly after Access. They are named after the pages
	// they open, so the menu never has two items called "Tools".
	if f("feature_groups") && (f("feature_portal") || f("feature_chat")) {
		out = append(out, group("catalogs", "Catalogs", "rectangle-history",
			when(f("feature_portal"), page("catalog-llms", "LLM catalogs", "/admin/catalogs/llms", r("catalogues"))),
			[]NavItem{page("catalog-data", "Data catalogs", "/admin/catalogs/data", r("data-catalogues"))},
			when(f("feature_chat"), page("catalog-tools", "Tool catalogs", "/admin/catalogs/tools", r("tool-catalogues"))),
		))
	}
	mcpServers := page("mcp-servers", "MCP servers", "/admin/mcp-servers", r("mcp-servers"))
	mcpServers.Exact = true
	out = append(out,
		group("llm-management", "LLM management", "microchip-ai",
			[]NavItem{
				page("llms", "LLM providers", "/admin/llms", r("llms")),
				page("model-prices", "Model prices", "/admin/model-prices", r("model-prices")),
				page("embedders", "Embedders", "/admin/embedders", r("embedders")),
			},
			when(f("feature_model_router"), page("model-routers", "Model Routers", "/admin/model-routers", r("model-routers"))),
			when(f("feature_semantic_router"), page("semantic-routers", "Semantic Routers", "/admin/semantic-routers", r("semantic-routers"))),
		),
		group("context-management", "Context management", "layer-group",
			[]NavItem{page("datasources", "Data sources", "/admin/datasources", r("datasources"))},
			when(f("feature_chat"), page("tools", "Tools", "/admin/tools", r("tools"))),
			when(f("feature_tyk_mcp"), mcpServers),
			when(in.enterprise, page("filters", "Filters", "/admin/filters", r("filters"))),
		),
	)
	// Apps have exactly one home: under "AI Portal" with the portal on, or
	// under "Apps & credentials" in gateway-only mode (no portal, no chat).
	if f("feature_gateway") && !f("feature_portal") && !f("feature_chat") {
		out = append(out, group("apps-credentials", "Apps & credentials", "grid-2-plus",
			[]NavItem{page("apps", "Apps", "/admin/apps", r("apps"))}))
	}
	if f("feature_portal") {
		out = append(out,
			group("ai-portal", "AI Portal", "display",
				[]NavItem{
					page("portal-apps", "Apps", "/admin/apps", r("apps")),
					page("edge-gateways", "Edge Gateways", "/admin/edge-gateways", r("edges")),
				},
				when(f("feature_tyk_mcp"), page("mcp-credentials", "MCP credentials", "/admin/mcp-credentials", r("mcp-credentials"))),
			),
			group("community", "Community", "puzzle-piece", []NavItem{
				page("submission-queue", "Submission Queue", "/admin/submissions", r("submissions")),
				page("attestation-templates", "Attestation Templates", "/admin/attestation-templates", r("attestation-templates")),
			}),
		)
	}
	// Governance only holds Enterprise pages, so the Community Edition
	// hides the whole group rather than showing an empty section.
	if in.enterprise {
		out = append(out, group("governance", "Governance", "shield",
			[]NavItem{
				page("compliance", "Compliance overview", "/admin/compliance", r("compliance")),
				page("audit", "Audit trail", "/admin/audit", r("audit")),
				page("metadata-schemas", "Metadata schemas", "/admin/metadata/schemas", r("metadata")),
				page("metadata-vocabularies", "Metadata vocabularies", "/admin/metadata/vocabularies", r("metadata")),
				page("metadata-compliance", "Metadata coverage", "/admin/metadata/coverage", r("metadata")),
			},
			when(f("feature_webhooks"), page("webhooks", "Webhooks", "/admin/webhooks", r("webhooks"))),
		))
	}
	// Plugin sections sit after Governance and before the system pages.
	out = append(out, pluginNav(in.plugins)...)
	out = append(out,
		group("settings", "Settings", "gear",
			[]NavItem{
				page("secrets", "Secrets", "/admin/secrets", r("secrets")),
				page("branding", "Branding", "/admin/branding", authz.Write("branding")),
			},
			when(f("feature_tyk_mcp"), page("tyk-connections", "Tyk Connections", "/admin/tyk-connections", r("tyk-connections"))),
		),
	)
	if f("feature_chat") {
		out = append(out, group("chat", "Chat", "message-lines", []NavItem{
			page("chats", "Chats", "/admin/chats", r("chats")),
			page("agents", "Agents", "/admin/agents", r("agents")),
			page("llm-settings", "Model call settings", "/admin/llm-settings", r("llm-settings")),
		}))
	}
	out = append(out, group("plugins", "Plugins", "screwdriver-wrench",
		[]NavItem{
			page("marketplace", "Marketplace", "/admin/marketplace", r("marketplace")),
			page("plugin-list", "Installed Plugins", "/admin/plugins", r("plugins")),
		},
		when(in.enterprise, page("marketplace-settings", "Marketplace Sources", "/admin/marketplace-settings", authz.Write("marketplace"))),
	))
	return out
}

// pluginNav turns plugin sidebar sections into menu groups, sorted by label
// (then id) so the order does not depend on installation order.
func pluginNav(sections []services.SidebarMenuItem) []NavItem {
	sorted := append([]services.SidebarMenuItem(nil), sections...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Label != sorted[j].Label {
			return sorted[i].Label < sorted[j].Label
		}
		return sorted[i].ID < sorted[j].ID
	})
	out := make([]NavItem, 0, len(sorted))
	for _, s := range sorted {
		perm := s.RequiredPermission
		if perm == "" {
			perm = string(authz.Execute("plugins"))
		}
		g := NavItem{ID: s.ID, Text: s.Label, Icon: "puzzle-piece", Path: s.Path, Title: s.Title, Permission: perm, PluginID: s.PluginID}
		for _, sub := range s.SubItems {
			subPerm := sub.RequiredPermission
			if subPerm == "" {
				subPerm = perm
			}
			// Exact-match a page whose path is a prefix of a sibling's, so
			// both do not highlight on the child route.
			exact := false
			for _, other := range s.SubItems {
				if other.ID != sub.ID && other.Path != "" && sub.Path != "" && strings.HasPrefix(other.Path, sub.Path+"/") {
					exact = true
				}
			}
			g.Items = append(g.Items, NavItem{ID: sub.ID, Text: sub.Text, Path: sub.Path, Exact: exact, Permission: subPerm, PluginID: s.PluginID})
		}
		out = append(out, g)
	}
	return out
}

// filterNav drops entries the user may not open, with the console drawer's
// rule (base-drawer/utils.js filterMenuItems): a page is kept when allowed;
// a group when at least one of its pages survives and its own permission,
// if any, is allowed; a group with no pages and no path of its own is
// dropped.
func filterNav(items []NavItem, allowed func(NavItem) bool) []NavItem {
	var out []NavItem
	for _, item := range items {
		if len(item.Items) > 0 {
			sub := filterNav(item.Items, allowed)
			if len(sub) == 0 || (item.Permission != "" && !allowed(item)) {
				continue
			}
			item.Items = sub
			out = append(out, item)
			continue
		}
		if item.Path == "" {
			// A group whose optional pages were all switched off.
			continue
		}
		if allowed(item) {
			out = append(out, item)
		}
	}
	return out
}

// @Summary Get the navigation manifest
// @Description The surfaces and admin menu entries the signed-in user may open, for a host that draws Studio's navigation itself
// @Tags system
// @Produce json
// @Success 200 {object} NavManifest
// @Router /common/nav [get]
func (a *API) getNavManifest(c *gin.Context) {
	v, _ := c.Get("user")
	u, ok := v.(*models.User)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	perms, err := authz.Permissions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "could not resolve permissions"}},
		})
		return
	}
	fs := a.featureSet()
	feature := func(name string) bool { v, _ := fs[name].(bool); return v }

	manifest := NavManifest{Surfaces: []NavItem{}, Admin: []NavItem{}}
	hasAdmin := !perms.IsEmpty()
	if hasAdmin {
		manifest.Surfaces = append(manifest.Surfaces, NavItem{ID: "admin", Text: "Admin", Icon: "gear", Path: "/admin"})
	}
	if u.ShowPortal && feature("feature_portal") {
		manifest.Surfaces = append(manifest.Surfaces, NavItem{ID: "portal", Text: "AI Portal", Icon: "display", Path: "/portal/dashboard"})
	}
	if u.ShowChat && feature("feature_chat") {
		manifest.Surfaces = append(manifest.Surfaces, NavItem{ID: "chat", Text: "Chat", Icon: "message-lines", Path: "/chat/dashboard"})
	}
	if !hasAdmin {
		c.JSON(http.StatusOK, manifest)
		return
	}

	allow := func(p string) bool { return authz.Can(c, authz.Permission(p)) }
	var plugins []services.SidebarMenuItem
	if a.service.PluginManifestService != nil {
		// A broken plugin manifest costs the plugin sections, not the menu.
		plugins, _ = a.service.PluginManifestService.GetSidebarMenuItemsFor(allow)
	}
	localSignIn := a.config == nil || a.config.LocalAccountsEnabled()
	in := navInputs{
		features:          feature,
		enterprise:        config.IsEnterprise(),
		identityProviders: localSignIn && a.showSSOConfig(u, perms),
		plugins:           plugins,
	}
	manifest.Admin = filterNav(adminNav(in), func(item NavItem) bool {
		return item.Permission == "" || allow(item.Permission)
	})
	if manifest.Admin == nil {
		manifest.Admin = []NavItem{}
	}
	c.JSON(http.StatusOK, manifest)
}
