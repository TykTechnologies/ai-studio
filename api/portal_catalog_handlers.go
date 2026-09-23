package api

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/pkg/modelmatch"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The portal's unified catalog (UX review D4 / F-09).
//
// Developers do not think in catalogs: they want to find something to build
// with. One endpoint therefore lists everything the caller can use -- LLM
// providers, data sources, tools and plugin resources -- in one item shape,
// so the portal can search, filter and sort across types and open a detail
// page per item without knowing which catalog holds what.
//
// Visibility is exactly the rule the per-catalog pages applied: the caller's
// teams -> their catalogs -> the active objects in them (the
// models.Accessible*Query queries, the same joins as User.GetAccessible*)
// and, for plugin resources, the teams' resource grants
// (services.AccessiblePluginResourceInstances). Nothing here widens that.
// The catalogs an item is reachable through are reported so the UI can
// explain why it is visible and offer them as a filter.

// Catalog item types.
const (
	CatalogItemLLM            = "llm"
	CatalogItemDatasource     = "datasource"
	CatalogItemTool           = "tool"
	CatalogItemPluginResource = "plugin_resource"
	CatalogItemMCPServer      = "mcp_server"
	CatalogItemModelRouter    = "model_router"
	CatalogItemSemanticRouter = "semantic_router"
)

// CatalogRef names one catalog an item is available through.
type CatalogRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CatalogResourceType describes the plugin resource type of a plugin resource item.
type CatalogResourceType struct {
	PluginID uint   `json:"plugin_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Icon     string `json:"icon,omitempty"`
	// AccessGrantedViaApp is the type-level answer to "does an App credential
	// grant access to these?"; the item carries the per-instance value.
	AccessGrantedViaApp bool `json:"access_granted_via_app"`
}

// CatalogModelInfo is one model an LLM provider serves, as far as the platform
// knows: the default model, the literal entries of the allow list, and every
// model in the vendor's price table that the allow list admits. Prices are
// reported per million tokens because that is how developers compare them.
type CatalogModelInfo struct {
	Name                  string   `json:"name"`
	IsDefault             bool     `json:"is_default"`
	InputPricePerMillion  *float64 `json:"input_price_per_million,omitempty"`
	OutputPricePerMillion *float64 `json:"output_price_per_million,omitempty"`
	Currency              string   `json:"currency,omitempty"`
}

// CatalogItemAttributes is the one shape every catalog item is rendered in.
type CatalogItemAttributes struct {
	Name             string `json:"name"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description,omitempty"`
	LogoURL          string `json:"logo_url,omitempty"`
	// Kind is the type-specific sub-classification: the vendor code of an LLM
	// provider, the store type of a data source, the protocol of a tool and
	// "<plugin id>:<slug>" for a plugin resource. KindLabel is set when the
	// backend knows a display name (plugin resource types); the UI maps the
	// built-in codes itself.
	Kind               string       `json:"kind"`
	KindLabel          string       `json:"kind_label,omitempty"`
	PrivacyScore       *int         `json:"privacy_score"`
	CommunitySubmitted bool         `json:"community_submitted"`
	Tags               []string     `json:"tags"`
	Catalogs           []CatalogRef `json:"catalogs"`
	CreatedAt          *time.Time   `json:"created_at"`
	UpdatedAt          *time.Time   `json:"updated_at"`
	// AccessGrantedViaApp says whether building an App is how a developer
	// gets to use this item. Always true for LLM providers, data sources and
	// tools; for plugin resources it is the type's resolved value with any
	// per-instance override applied. When false the portal shows no
	// "Build app" action.
	//
	// PortalDetailURL is the providing plugin's own page for the item, set
	// whenever the type declared portal_detail_path. When access is not
	// granted through an App it replaces the built-in detail page as the
	// item's detail view; otherwise it is a secondary link next to
	// "Build app".
	AccessGrantedViaApp bool   `json:"access_granted_via_app"`
	PortalDetailURL     string `json:"portal_detail_url,omitempty"`

	// LLM providers
	DefaultModel  string   `json:"default_model,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	// Detail responses only: the known models and the provider's metadata
	// (secrets redacted, as on the admin API).
	Models   []CatalogModelInfo     `json:"models,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Data sources
	EmbedVendor string `json:"embed_vendor,omitempty"`
	EmbedModel  string `json:"embed_model,omitempty"`

	// Tools. A tool is served by the AI Studio gateway; the access methods say
	// how an App may reach it, and each URL is present only when its method is
	// switched on. A tool with neither method on is chat only and is not in
	// the catalog at all.
	Operations        []string `json:"operations,omitempty"`
	RESTAccessEnabled *bool    `json:"rest_access_enabled,omitempty"`
	MCPAccessEnabled  *bool    `json:"mcp_access_enabled,omitempty"`
	RESTEndpointURL   string   `json:"rest_endpoint_url,omitempty"`
	MCPEndpointURL    string   `json:"mcp_endpoint_url,omitempty"`

	// Plugin resources
	ResourceType *CatalogResourceType `json:"resource_type,omitempty"`

	// MCP servers (Tyk-managed): how a client reaches the proxy and what it
	// offers. Brokerable says whether an App credential can be minted for it.
	AuthMode     string                               `json:"auth_mode,omitempty"`
	AuthHeader   string                               `json:"auth_header,omitempty"`
	EndpointURL  string                               `json:"endpoint_url,omitempty"`
	EndpointURLs map[string]string                    `json:"endpoint_urls,omitempty"`
	Primitives   []models.MCPPrimitive                `json:"primitives,omitempty"`
	GatewayTags  []string                             `json:"gateway_tags,omitempty"`
	Brokerable   *bool                                `json:"brokerable,omitempty"`
	OAuth        *models.MCPProtectedResourceMetadata `json:"oauth,omitempty"`

	// Model Routers: the slug, the "{slug}/{model}" strings a client sends
	// to the unified ingress, and (detail only) the LLMs it can route to.
	RouterSlug   string             `json:"router_slug,omitempty"`
	RouterModels []string           `json:"router_models,omitempty"`
	RouterLLMs   []CatalogRouterLLM `json:"router_llms,omitempty"`
	// RouterRoutes are a Semantic Router's routes (name and description; the
	// examples and keywords that pick them are configuration and stay out).
	RouterRoutes []CatalogRouterRoute `json:"router_routes,omitempty"`
}

// CatalogItem is one entry of the unified catalog.
type CatalogItem struct {
	Type       string                `json:"type"`
	ID         string                `json:"id"`
	Attributes CatalogItemAttributes `json:"attributes"`
	// Portal-visible governed metadata (Enterprise), display-ready.
	GovernedMetadata interface{} `json:"governed_metadata,omitempty"`
}

// CatalogFilterOption is one catalog the caller can filter by.
type CatalogFilterOption struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CatalogListMeta carries the page position and the facets the browse page
// builds its filters from. Total is the number of items matching the query;
// Counts, Kinds, Catalogs and ResourceTypes describe the caller's whole
// accessible set so the filter controls do not shrink as filters are applied.
type CatalogListMeta struct {
	Total         int                   `json:"total"`
	Page          int                   `json:"page"`
	PageSize      int                   `json:"page_size"`
	TotalPages    int                   `json:"total_pages"`
	Counts        map[string]int        `json:"counts"`
	Kinds         []CatalogKindFacet    `json:"kinds"`
	Catalogs      []CatalogFilterOption `json:"catalogs"`
	ResourceTypes []CatalogResourceType `json:"resource_types"`
}

// CatalogListResponse is the body of GET /common/catalog.
type CatalogListResponse struct {
	Data []CatalogItem   `json:"data"`
	Meta CatalogListMeta `json:"meta"`
}

// CatalogItemResponse is the body of the per-item detail endpoints.
type CatalogItemResponse struct {
	Data CatalogItem `json:"data"`
}

func intPtr(v int) *int { return &v }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func uintID(id uint) string { return strconv.FormatUint(uint64(id), 10) }

// --- the caller's catalogues, per type ------------------------------------

// accessibleCatalogues is the caller's catalogues of one type: the filter
// options, and the id -> name map used to label an item's memberships.
type accessibleCatalogues struct {
	ids     []uint
	names   map[uint]string
	options []CatalogFilterOption
}

func (a *API) accessibleCataloguesFor(typ string, user *models.User) (accessibleCatalogues, error) {
	out := accessibleCatalogues{names: map[uint]string{}, options: []CatalogFilterOption{}}
	add := func(id uint, name string) {
		out.ids = append(out.ids, id)
		out.names[id] = cleanText(name)
		out.options = append(out.options, CatalogFilterOption{Type: typ, ID: uintID(id), Name: cleanText(name)})
	}
	switch typ {
	case CatalogItemLLM:
		cats, err := user.GetAccessibleCatalogues(a.service.DB)
		if err != nil {
			return out, err
		}
		for _, cat := range cats {
			add(cat.ID, cat.Name)
		}
	case CatalogItemDatasource:
		cats, err := user.GetAccessibleDataCatalogues(a.service.DB)
		if err != nil {
			return out, err
		}
		for _, cat := range cats {
			add(cat.ID, cat.Name)
		}
	case CatalogItemTool:
		cats, err := user.GetAccessibleToolCatalogues(a.service.DB)
		if err != nil {
			return out, err
		}
		for _, cat := range cats {
			add(cat.ID, cat.Name)
		}
	}
	return out, nil
}

func catalogRefs(ids []uint, names map[uint]string) []CatalogRef {
	refs := make([]CatalogRef, 0, len(ids))
	for _, id := range ids {
		if name, ok := names[id]; ok {
			refs = append(refs, CatalogRef{ID: uintID(id), Name: name})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs
}

// --- loading database-backed items -----------------------------------------

// catalogPage describes which rows to load: every match, or one SQL page.
type catalogPage struct {
	order  string
	offset int
	limit  int
}

// loadCatalogItems runs a type's base query with the given scope and
// returns the matching rows as catalog items, with their catalog
// memberships and governed metadata attached. Memberships and governed
// metadata are one query each for the rows loaded.
func (a *API) loadCatalogItems(user *models.User, src *catalogSource, scope func(*gorm.DB) *gorm.DB, page catalogPage) ([]CatalogItem, error) {
	db := a.service.DB
	query := src.base(db, user.ID).Scopes(scope).Group(src.table + ".id")
	if page.order != "" {
		query = query.Order(page.order)
	}
	if page.limit > 0 {
		query = query.Offset(page.offset).Limit(page.limit)
	}

	var items []CatalogItem
	var idOf func(i int) uint
	var memberships func(db *gorm.DB, catalogueIDs []uint) (map[uint][]uint, error)
	var objectType string

	switch src.typ {
	case CatalogItemLLM:
		var llms []models.LLM
		if err := query.Find(&llms).Error; err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(llms))
		for i := range llms {
			items[i] = llmCatalogItem(&llms[i])
		}
		idOf = func(i int) uint { return llms[i].ID }
		memberships, objectType = models.LLMCatalogueMemberships, models.GovernedObjectTypeLLM
	case CatalogItemDatasource:
		var datasources []models.Datasource
		if err := query.Preload("Tags").Find(&datasources).Error; err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(datasources))
		for i := range datasources {
			items[i] = datasourceCatalogItem(&datasources[i])
		}
		idOf = func(i int) uint { return datasources[i].ID }
		memberships, objectType = models.DatasourceCatalogueMemberships, models.GovernedObjectTypeDatasource
	case CatalogItemTool:
		var tools []models.Tool
		if err := query.Find(&tools).Error; err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(tools))
		for i := range tools {
			items[i] = toolCatalogItem(&tools[i])
		}
		idOf = func(i int) uint { return tools[i].ID }
		memberships, objectType = models.ToolCatalogueMemberships, models.GovernedObjectTypeTool
	case CatalogItemMCPServer:
		var servers []models.MCPServer
		if err := query.Find(&servers).Error; err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(servers))
		for i := range servers {
			items[i] = mcpServerCatalogItem(&servers[i])
		}
		idOf = func(i int) uint { return servers[i].ID }
		memberships, objectType = models.MCPServerCatalogueMemberships, models.GovernedObjectTypeMCPServer
	case CatalogItemModelRouter:
		var routers []models.ModelRouter
		if err := query.Preload("Pools.Vendors.Mappings").Find(&routers).Error; err != nil {
			return nil, err
		}
		ids := make([]uint, len(routers))
		for i := range routers {
			ids[i] = routers[i].ID
		}
		privacy, err := models.ModelRouterPrivacyScores(db, ids)
		if err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(routers))
		for i := range routers {
			items[i] = modelRouterCatalogItem(&routers[i], privacy)
		}
		idOf = func(i int) uint { return routers[i].ID }
		memberships = models.ModelRouterCatalogueMemberships
	case CatalogItemSemanticRouter:
		var routers []models.SemanticRouter
		if err := query.Find(&routers).Error; err != nil {
			return nil, err
		}
		ids := make([]uint, len(routers))
		for i := range routers {
			ids[i] = routers[i].ID
		}
		privacy, err := models.SemanticRouterPrivacyScores(db, ids)
		if err != nil {
			return nil, err
		}
		items = make([]CatalogItem, len(routers))
		for i := range routers {
			items[i] = semanticRouterCatalogItem(&routers[i], privacy)
		}
		idOf = func(i int) uint { return routers[i].ID }
		memberships = models.SemanticRouterCatalogueMemberships
	default:
		return nil, nil
	}
	if len(items) == 0 {
		return []CatalogItem{}, nil
	}

	// MCP servers live in tool catalogues, so their memberships are labelled
	// from the same list a tool's are.
	catalogues, err := a.accessibleCataloguesFor(src.catalogueFamily(), user)
	if err != nil {
		return nil, err
	}
	membership := map[uint][]uint{}
	if memberships != nil {
		if membership, err = memberships(db, catalogues.ids); err != nil {
			return nil, err
		}
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	governed := a.governedMetadataFor(objectType, ids)
	for i := range items {
		items[i].Attributes.Catalogs = catalogRefs(membership[idOf(i)], catalogues.names)
		if governed != nil {
			items[i].GovernedMetadata = a.portalGovernedView(objectType, governed[items[i].ID])
		}
	}
	return items, nil
}

func llmCatalogItem(llm *models.LLM) CatalogItem {
	return CatalogItem{Type: CatalogItemLLM, ID: uintID(llm.ID), Attributes: CatalogItemAttributes{
		Name:                cleanText(llm.Name),
		ShortDescription:    cleanText(llm.ShortDescription),
		LongDescription:     cleanText(llm.LongDescription),
		LogoURL:             llm.LogoURL,
		Kind:                string(llm.Vendor),
		PrivacyScore:        intPtr(llm.PrivacyScore),
		Tags:                []string{},
		Catalogs:            []CatalogRef{},
		CreatedAt:           timePtr(llm.CreatedAt),
		UpdatedAt:           timePtr(llm.UpdatedAt),
		AccessGrantedViaApp: true,
		DefaultModel:        cleanText(llm.DefaultModel),
		AllowedModels:       cleanTexts(llm.AllowedModels),
	}}
}

func datasourceCatalogItem(ds *models.Datasource) CatalogItem {
	tags := make([]string, 0, len(ds.Tags))
	for _, tag := range ds.Tags {
		tags = append(tags, cleanText(tag.Name))
	}
	return CatalogItem{Type: CatalogItemDatasource, ID: uintID(ds.ID), Attributes: CatalogItemAttributes{
		Name:                cleanText(ds.Name),
		ShortDescription:    cleanText(ds.ShortDescription),
		LongDescription:     cleanText(ds.LongDescription),
		LogoURL:             ds.Icon,
		Kind:                ds.DBSourceType,
		PrivacyScore:        intPtr(ds.PrivacyScore),
		CommunitySubmitted:  ds.CommunitySubmitted,
		Tags:                tags,
		Catalogs:            []CatalogRef{},
		CreatedAt:           timePtr(ds.CreatedAt),
		UpdatedAt:           timePtr(ds.UpdatedAt),
		AccessGrantedViaApp: true,
		EmbedVendor:         string(ds.EmbedVendor),
		EmbedModel:          cleanText(ds.EmbedModel),
	}}
}

func toolCatalogItem(tool *models.Tool) CatalogItem {
	access := toolGatewayAccess(tool)
	item := CatalogItem{Type: CatalogItemTool, ID: uintID(tool.ID), Attributes: CatalogItemAttributes{
		Name:               cleanText(tool.Name),
		ShortDescription:   cleanText(tool.Description),
		Kind:               tool.ToolType,
		PrivacyScore:       intPtr(tool.PrivacyScore),
		CommunitySubmitted: tool.CommunitySubmitted,
		Tags:               []string{},
		Catalogs:           []CatalogRef{},
		CreatedAt:          timePtr(tool.CreatedAt),
		UpdatedAt:          timePtr(tool.UpdatedAt),
		// Always true for a listed tool: the catalog only lists tools an App
		// can reach (models.AccessibleToolQuery).
		AccessGrantedViaApp: access.AppGrantable,
		Operations:          cleanTexts(tool.GetOperations()),
		RESTAccessEnabled:   &access.RESTAccessEnabled,
		MCPAccessEnabled:    &access.MCPAccessEnabled,
	}}
	if access.RESTAccessEnabled {
		item.Attributes.RESTEndpointURL = access.RESTEndpointURL
	}
	if access.MCPAccessEnabled {
		item.Attributes.MCPEndpointURL = access.MCPEndpointURL
	}
	return item
}

// portalDetailURL expands a resource type's portal detail path template for
// one instance. "{id}" is replaced with the path-escaped instance ID; a
// template without the placeholder is returned as is. Templates are
// validated to be same-origin paths at registration
// (services.ValidatePortalDetailPath).
func portalDetailURL(template, instanceID string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	return strings.ReplaceAll(template, "{id}", url.PathEscape(instanceID))
}

// applyPluginResourceAccess sets a plugin resource item's access class (the
// type's resolved value with the instance override applied) and its link to
// the providing plugin's page. The link is set for both access classes: a
// plugin page can show what the built-in detail page cannot (schema fields,
// gated values, request flows) whether or not an App is the way in.
func applyPluginResourceAccess(attrs *CatalogItemAttributes, t *models.PluginResourceType, instanceID string, instanceOverride *bool) {
	attrs.AccessGrantedViaApp = models.EffectiveInstanceAccessGrantedViaApp(t.AccessGrantedViaApp, instanceOverride)
	attrs.PortalDetailURL = portalDetailURL(t.PortalDetailPath, instanceID)
}

// countCatalogItems counts the type's matching objects.
func (a *API) countCatalogItems(user *models.User, src *catalogSource, scope func(*gorm.DB) *gorm.DB) (int, error) {
	var n int64
	err := src.base(a.service.DB, user.ID).Scopes(scope).Distinct(src.table + ".id").Count(&n).Error
	return int(n), err
}

// kindFacetsFor counts the type's accessible objects per kind; the sum is
// the type's total.
func (a *API) kindFacetsFor(user *models.User, src *catalogSource) ([]CatalogKindFacet, int, error) {
	var rows []struct {
		Kind  string
		Count int
	}
	err := src.base(a.service.DB, user.ID).
		Select(src.kindCol + " AS kind, COUNT(DISTINCT " + src.table + ".id) AS count").
		Group(src.kindCol).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	total := 0
	facets := make([]CatalogKindFacet, 0, len(rows))
	for _, row := range rows {
		total += row.Count
		if row.Kind == "" {
			continue
		}
		facets = append(facets, CatalogKindFacet{Type: src.typ, Kind: row.Kind, Label: catalogVendorNames[row.Kind], Count: row.Count})
	}
	return facets, total, nil
}

// --- plugin resources --------------------------------------------------------

// pluginResourceItems turns the plugin resource types the caller may use
// into catalog items (all of them; the query is applied by the caller). The
// visibility rule lives in services.AccessiblePluginResourceInstances; team
// managers see every instance.
func (a *API) pluginResourceItems(c *gin.Context, user *models.User, only *services.PluginResourceTypeKey) ([]CatalogItem, []CatalogResourceType, error) {
	seeAll := authz.Can(c, authz.Write("groups"))
	resourceTypes, err := a.service.AccessiblePluginResourceInstances(user.ID, seeAll, only)
	if err != nil {
		// Fail closed: an unknown grant set means no plugin resources.
		return []CatalogItem{}, []CatalogResourceType{}, nil
	}
	items := []CatalogItem{}
	types := make([]CatalogResourceType, 0, len(resourceTypes))
	for _, rt := range resourceTypes {
		ref := CatalogResourceType{PluginID: rt.Type.PluginID, Slug: rt.Type.Slug, Name: sanitizeString(rt.Type.Name), Icon: sanitizeString(rt.Type.Icon), AccessGrantedViaApp: rt.Type.AccessGrantedViaApp}
		types = append(types, ref)
		kind := uintID(rt.Type.PluginID) + ":" + rt.Type.Slug
		objectType := models.PluginResourceObjectType(rt.Type.PluginID, rt.Type.Slug)
		var governed map[string]*models.ObjectMetadata
		if rt.Type.SupportsMetadata && len(rt.Instances) > 0 {
			ids := make([]string, 0, len(rt.Instances))
			for _, inst := range rt.Instances {
				ids = append(ids, inst.Id)
			}
			governed = a.governedMetadataFor(objectType, ids)
		}
		for _, inst := range rt.Instances {
			refCopy := ref
			item := CatalogItem{Type: CatalogItemPluginResource, ID: inst.Id, Attributes: CatalogItemAttributes{
				Name:             sanitizeString(inst.Name),
				ShortDescription: sanitizeString(inst.Description),
				Kind:             kind,
				KindLabel:        refCopy.Name,
				Tags:             []string{},
				Catalogs:         []CatalogRef{},
				ResourceType:     &refCopy,
			}}
			if rt.Type.HasPrivacyScore {
				item.Attributes.PrivacyScore = intPtr(int(inst.PrivacyScore))
			}
			applyPluginResourceAccess(&item.Attributes, &rt.Type, inst.Id, inst.AccessGrantedViaApp)
			if governed != nil {
				item.GovernedMetadata = a.portalGovernedView(objectType, governed[inst.Id])
			}
			items = append(items, item)
		}
	}
	return items, types, nil
}

// --- handlers ----------------------------------------------------------------

// portalUser reads the authenticated user or writes a 401.
func portalUser(c *gin.Context) (*models.User, bool) {
	value, exists := c.Get("user")
	if !exists {
		simpleError(c, http.StatusUnauthorized, "Unauthorized", "User not found in context")
		return nil, false
	}
	user, ok := value.(*models.User)
	if !ok || user == nil {
		simpleError(c, http.StatusUnauthorized, "Unauthorized", "User not found in context")
		return nil, false
	}
	return user, true
}

// getPortalCatalog godoc
// @Summary The portal's unified catalog
// @Description One page of the LLM providers, data sources, tools and plugin resources the authenticated user can build an app with, in one item shape, with the catalogs each is available through. Search (q), filters (type, kind, privacy, catalog, community), sort and paging (page, page_size) are applied on the server; meta carries the facets for the filter controls. Visibility follows the user's teams and their catalogs.
// @Tags common
// @Produce json
// @Param q query string false "Search terms (all must match)"
// @Param type query string false "llm | datasource | tool | plugin_resource"
// @Param kind query string false "Vendor code, store type, protocol or <plugin id>:<slug>"
// @Param privacy query string false "public | internal | confidential | restricted"
// @Param catalog query string false "<type>:<catalog id>"
// @Param community query bool false "Community submissions only"
// @Param sort query string false "newest (default) | name | privacy_asc | privacy_desc"
// @Param page query int false "Page number (1-based)"
// @Param page_size query int false "Items per page (default 25, max 100)"
// @Success 200 {object} CatalogListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/catalog [get]
func (a *API) getPortalCatalog(c *gin.Context) {
	user, ok := portalUser(c)
	if !ok {
		return
	}
	query, ok := parseCatalogQuery(c)
	if !ok {
		return
	}

	// Facets over the whole accessible set: aggregates for the database
	// types, the plugin instances (which the plugins list in full anyway).
	counts := map[string]int{CatalogItemLLM: 0, CatalogItemDatasource: 0, CatalogItemTool: 0, CatalogItemPluginResource: 0, CatalogItemMCPServer: 0, CatalogItemModelRouter: 0, CatalogItemSemanticRouter: 0}
	kinds := []CatalogKindFacet{}
	catalogs := []CatalogFilterOption{}
	for i := range catalogSources {
		src := &catalogSources[i]
		facets, total, err := a.kindFacetsFor(user, src)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		counts[src.typ] = total
		kinds = append(kinds, facets...)
		cats, err := a.accessibleCataloguesFor(src.typ, user)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		catalogs = append(catalogs, cats.options...)
	}
	pluginItems, resourceTypes, err := a.pluginResourceItems(c, user, nil)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	counts[CatalogItemPluginResource] = len(pluginItems)
	pluginKinds := map[string]*CatalogKindFacet{}
	for i := range pluginItems {
		a := &pluginItems[i].Attributes
		if facet, ok := pluginKinds[a.Kind]; ok {
			facet.Count++
			continue
		}
		pluginKinds[a.Kind] = &CatalogKindFacet{Type: CatalogItemPluginResource, Kind: a.Kind, Label: a.KindLabel, Count: 1}
	}
	for _, facet := range pluginKinds {
		kinds = append(kinds, *facet)
	}
	sortKindFacets(kinds)

	meta := CatalogListMeta{
		Page:          query.Page,
		PageSize:      query.PageSize,
		Counts:        counts,
		Kinds:         kinds,
		Catalogs:      catalogs,
		ResourceTypes: resourceTypes,
	}

	// One database type: filter, order and page in SQL.
	if src := catalogSourceFor(query.Type); src != nil {
		if !src.applies(query) {
			meta.Total, meta.TotalPages = 0, 1
			c.JSON(http.StatusOK, CatalogListResponse{Data: []CatalogItem{}, Meta: meta})
			return
		}
		scope := src.scope(query)
		total, err := a.countCatalogItems(user, src, scope)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		items, err := a.loadCatalogItems(user, src, scope, catalogPage{
			order:  src.order(query.Sort),
			offset: (query.Page - 1) * query.PageSize,
			limit:  query.PageSize,
		})
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		meta.Total, meta.TotalPages = total, totalPagesFor(total, query.PageSize)
		c.JSON(http.StatusOK, CatalogListResponse{Data: items, Meta: meta})
		return
	}

	// Every type, or plugin resources. The database types are combined with
	// UNION ALL and counted, ordered and paged by the database; plugin
	// resources (listed by their plugins over RPC) are matched here and come
	// after the database items, so a page is the database page followed by
	// whatever plugin resources fall into the same window.
	sources := []*catalogSource{}
	for i := range catalogSources {
		if catalogSources[i].applies(query) {
			sources = append(sources, &catalogSources[i])
		}
	}
	pluginMatched := []CatalogItem{}
	if query.Type == "" || query.Type == CatalogItemPluginResource {
		for i := range pluginItems {
			if query.matches(&pluginItems[i]) {
				pluginMatched = append(pluginMatched, pluginItems[i])
			}
		}
		sortCatalogItems(pluginMatched, query.Sort)
	}
	offset := (query.Page - 1) * query.PageSize
	refs, dbTotal, err := unionPage(a.service.DB, user.ID, query, sources, offset, query.PageSize)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	pageItems, err := a.loadCatalogRefs(user, refs)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	if remaining := query.PageSize - len(pageItems); remaining > 0 && len(pluginMatched) > 0 {
		start := offset - dbTotal
		if start < 0 {
			start = 0
		}
		if start < len(pluginMatched) {
			end := start + remaining
			if end > len(pluginMatched) {
				end = len(pluginMatched)
			}
			pageItems = append(pageItems, pluginMatched[start:end]...)
		}
	}
	total := dbTotal + len(pluginMatched)
	meta.Total, meta.TotalPages = total, totalPagesFor(total, query.PageSize)
	c.JSON(http.StatusOK, CatalogListResponse{Data: pageItems, Meta: meta})
}

// loadCatalogRefs loads the items a union page refers to, one query per
// type present, and returns them in the page's order.
func (a *API) loadCatalogRefs(user *models.User, refs []unionRef) ([]CatalogItem, error) {
	items := []CatalogItem{}
	if len(refs) == 0 {
		return items, nil
	}
	idsByType := map[string][]uint{}
	for _, ref := range refs {
		idsByType[ref.ItemType] = append(idsByType[ref.ItemType], ref.ItemID)
	}
	byKey := map[string]CatalogItem{}
	for typ, ids := range idsByType {
		src := catalogSourceFor(typ)
		if src == nil {
			continue
		}
		loaded, err := a.loadCatalogItems(user, src, func(db *gorm.DB) *gorm.DB {
			return db.Where(src.table+".id IN ?", ids)
		}, catalogPage{})
		if err != nil {
			return nil, err
		}
		for _, item := range loaded {
			byKey[item.Type+":"+item.ID] = item
		}
	}
	for _, ref := range refs {
		if item, ok := byKey[ref.ItemType+":"+uintID(ref.ItemID)]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

// findCatalogItem answers a detail request for a database-backed type: the
// one item if the caller can see it, otherwise 404. The base query carries
// the visibility rule, so a single-row query is the check. Anything outside
// the caller's visibility is "not found" rather than "forbidden" so the
// endpoint never confirms that an id exists.
func (a *API) findCatalogItem(c *gin.Context, typ, id string) (*CatalogItem, bool) {
	user, ok := portalUser(c)
	if !ok {
		return nil, false
	}
	src := catalogSourceFor(typ)
	numericID, err := strconv.ParseUint(id, 10, 64)
	if src == nil || err != nil {
		simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
		return nil, false
	}
	items, err := a.loadCatalogItems(user, src, func(db *gorm.DB) *gorm.DB {
		return db.Where(src.table+".id = ?", numericID)
	}, catalogPage{})
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return nil, false
	}
	if len(items) == 0 {
		simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
		return nil, false
	}
	return &items[0], true
}

// getPortalCatalogLLM godoc
// @Summary One LLM provider from the portal catalog
// @Description The catalog entry for an LLM provider the user can see, with the models it serves and its metadata.
// @Tags common
// @Produce json
// @Param id path int true "LLM ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/llms/{id} [get]
func (a *API) getPortalCatalogLLM(c *gin.Context) {
	item, ok := a.findCatalogItem(c, CatalogItemLLM, c.Param("id"))
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(item.ID, 10, 64)
	llm, err := a.service.GetLLMByID(uint(id))
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	item.Attributes.Metadata = redactMetadataSecrets(llm.Metadata)
	prices, _ := a.service.GetModelPricesByVendor(string(llm.Vendor))
	item.Attributes.Models = catalogModels(llm, prices)
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
}

// literalModelPattern matches allow-list entries that are plain model names
// rather than regular expressions, so they can be listed as models.
var literalModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)

// catalogModels lists the models an LLM provider is known to serve: the
// default model, the allow list's literal entries, and the vendor's priced
// models that pass the allow list (an empty allow list admits everything).
func catalogModels(llm *models.LLM, prices models.ModelPrices) []CatalogModelInfo {
	priceByName := make(map[string]models.ModelPrice, len(prices))
	for _, price := range prices {
		priceByName[price.ModelName] = price
	}
	seen := make(map[string]bool)
	out := make([]CatalogModelInfo, 0)
	add := func(name string, isDefault bool) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		info := CatalogModelInfo{Name: name, IsDefault: isDefault}
		// A row with both prices at zero is a placeholder, not a free model.
		if price, ok := priceByName[name]; ok && (price.CPIT > 0 || price.CPT > 0) {
			in := price.CPIT * 1_000_000
			outPrice := price.CPT * 1_000_000
			info.InputPricePerMillion = &in
			info.OutputPricePerMillion = &outPrice
			info.Currency = price.Currency
		}
		out = append(out, info)
	}
	add(llm.DefaultModel, true)
	for _, pattern := range llm.AllowedModels {
		if literalModelPattern.MatchString(pattern) {
			add(pattern, false)
		}
	}
	priced := make([]string, 0, len(prices))
	for _, price := range prices {
		priced = append(priced, price.ModelName)
	}
	sort.Strings(priced)
	for _, name := range priced {
		if modelmatch.Allowed(llm.AllowedModels, name) {
			add(name, false)
		}
	}
	return out
}

// getPortalCatalogDatasource godoc
// @Summary One data source from the portal catalog
// @Tags common
// @Produce json
// @Param id path int true "Data source ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/datasources/{id} [get]
func (a *API) getPortalCatalogDatasource(c *gin.Context) {
	item, ok := a.findCatalogItem(c, CatalogItemDatasource, c.Param("id"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
}

// getPortalCatalogTool godoc
// @Summary One tool from the portal catalog
// @Tags common
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/tools/{id} [get]
func (a *API) getPortalCatalogTool(c *gin.Context) {
	item, ok := a.findCatalogItem(c, CatalogItemTool, c.Param("id"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
}

// getPortalCatalogPluginResource godoc
// @Summary One plugin resource from the portal catalog
// @Tags common
// @Produce json
// @Param plugin_id path int true "Plugin ID"
// @Param slug path string true "Resource type slug"
// @Param id path string true "Instance ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/resources/{plugin_id}/{slug}/{id} [get]
func (a *API) getPortalCatalogPluginResource(c *gin.Context) {
	user, ok := portalUser(c)
	if !ok {
		return
	}
	pluginID, err := strconv.ParseUint(c.Param("plugin_id"), 10, 64)
	if err != nil {
		simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
		return
	}
	// One type's instances only; the plugin lists them in one RPC.
	items, _, err := a.pluginResourceItems(c, user, &services.PluginResourceTypeKey{PluginID: uint(pluginID), Slug: c.Param("slug")})
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	for i := range items {
		if items[i].ID == c.Param("id") {
			c.JSON(http.StatusOK, CatalogItemResponse{Data: items[i]})
			return
		}
	}
	simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
}

// AppUsageSummary is one app's budget position and recent activity for the
// portal overview.
type AppUsageSummary struct {
	AppID           string     `json:"app_id"`
	CurrentSpend    float64    `json:"current_spend"`
	MonthlyBudget   *float64   `json:"monthly_budget"`
	Percentage      *float64   `json:"percentage"`
	BudgetStartDate time.Time  `json:"budget_start_date"`
	LastAccessAt    *time.Time `json:"last_access_at"`
	Requests30d     int64      `json:"requests_30d"`
}

// AppUsageSummaryResponse is the body of GET /common/apps/usage-summary.
type AppUsageSummaryResponse struct {
	Data map[string]AppUsageSummary `json:"data"`
	// SpendTracked is false in Community Edition, where spending is not
	// recorded and current_spend is always zero.
	SpendTracked bool `json:"spend_tracked"`
}

// getUserAppsUsageSummary godoc
// @Summary Budget and activity summary for the user's apps
// @Description Spend against budget, last gateway access and 30-day request count for every app the authenticated user owns, in one call.
// @Tags common
// @Produce json
// @Success 200 {object} AppUsageSummaryResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/apps/usage-summary [get]
func (a *API) getUserAppsUsageSummary(c *gin.Context) {
	user, ok := portalUser(c)
	if !ok {
		return
	}
	apps, _, _, err := a.service.ListAppsByUserID(user.ID, 0, 0, true, "")
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	now := time.Now()
	appIDs := make([]uint, 0, len(apps))
	windows := make([]analytics.AppSpendWindow, 0, len(apps))
	starts := make(map[uint]time.Time, len(apps))
	for _, app := range apps {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		if app.BudgetStartDate != nil {
			start = *app.BudgetStartDate
		}
		appIDs = append(appIDs, app.ID)
		windows = append(windows, analytics.AppSpendWindow{AppID: app.ID, Start: start})
		starts[app.ID] = start
	}

	// Two grouped queries for every app: activity, and spend since each
	// app's budget start (the budget service's own figure, computed the same
	// way -- SUM(cost)/10000 over llm_chat_records -- but for all apps at
	// once; spending is an Enterprise feature, so Community Edition reports
	// zero and says so).
	activity, err := analytics.GetAppActivity(a.service.DB, appIDs, now.AddDate(0, 0, -30))
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	spendTracked := budget.IsEnterpriseAvailable()
	spending := map[uint]float64{}
	if spendTracked {
		spending, err = analytics.GetAppSpending(a.service.DB, windows, now)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
	}

	data := make(map[string]AppUsageSummary, len(apps))
	for _, app := range apps {
		spent := spending[app.ID]
		summary := AppUsageSummary{
			AppID:           uintID(app.ID),
			CurrentSpend:    spent,
			MonthlyBudget:   app.MonthlyBudget,
			BudgetStartDate: starts[app.ID],
		}
		if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
			pct := (spent / *app.MonthlyBudget) * 100
			summary.Percentage = &pct
		}
		if act := activity[app.ID]; act != nil {
			summary.LastAccessAt = act.LastAccessAt
			summary.Requests30d = act.Requests
		}
		data[summary.AppID] = summary
	}
	c.JSON(http.StatusOK, AppUsageSummaryResponse{Data: data, SpendTracked: spendTracked})
}
