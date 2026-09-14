package api

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/pkg/modelmatch"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
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
// models.User.GetAccessible* queries) and, for plugin resources, the teams'
// resource grants. Nothing here widens that. The catalogs an item is
// reachable through are reported so the UI can explain why it is visible and
// offer them as a filter.

// Catalog item types.
const (
	CatalogItemLLM            = "llm"
	CatalogItemDatasource     = "datasource"
	CatalogItemTool           = "tool"
	CatalogItemPluginResource = "plugin_resource"
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

	// Tools
	Operations []string `json:"operations,omitempty"`

	// Plugin resources
	ResourceType *CatalogResourceType `json:"resource_type,omitempty"`
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

// CatalogListMeta carries what the browse page needs to build its filters.
type CatalogListMeta struct {
	Total         int                   `json:"total"`
	Counts        map[string]int        `json:"counts"`
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

// portalCatalog is everything one caller can see, assembled once per request.
type portalCatalog struct {
	Items         []CatalogItem
	Catalogs      []CatalogFilterOption
	ResourceTypes []CatalogResourceType
}

func intPtr(v int) *int { return &v }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func catalogRefs(ids []uint, names map[uint]string) []CatalogRef {
	refs := make([]CatalogRef, 0, len(ids))
	for _, id := range ids {
		if name, ok := names[id]; ok {
			refs = append(refs, CatalogRef{ID: strconv.FormatUint(uint64(id), 10), Name: name})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs
}

// buildPortalCatalog assembles the caller's catalog. Each type is one
// accessible-objects query, one catalog membership query and one governed
// metadata lookup; plugin resources are fetched from their plugins as the
// AppBuilder does.
func (a *API) buildPortalCatalog(c *gin.Context, user *models.User) (*portalCatalog, error) {
	db := a.service.DB
	out := &portalCatalog{Items: []CatalogItem{}, Catalogs: []CatalogFilterOption{}, ResourceTypes: []CatalogResourceType{}}

	// --- LLM providers ---
	llmCatalogues, err := user.GetAccessibleCatalogues(db)
	if err != nil {
		return nil, err
	}
	llmCatIDs := make([]uint, 0, len(llmCatalogues))
	llmCatNames := make(map[uint]string, len(llmCatalogues))
	for _, cat := range llmCatalogues {
		llmCatIDs = append(llmCatIDs, cat.ID)
		llmCatNames[cat.ID] = cat.Name
		out.Catalogs = append(out.Catalogs, CatalogFilterOption{Type: CatalogItemLLM, ID: strconv.FormatUint(uint64(cat.ID), 10), Name: cat.Name})
	}
	llms, err := user.GetAccessibleLLMs(db)
	if err != nil {
		return nil, err
	}
	llmMembership, err := models.LLMCatalogueMemberships(db, llmCatIDs)
	if err != nil {
		return nil, err
	}
	llmIDs := make([]string, 0, len(llms))
	for _, llm := range llms {
		llmIDs = append(llmIDs, strconv.FormatUint(uint64(llm.ID), 10))
	}
	llmGoverned := a.governedMetadataFor(models.GovernedObjectTypeLLM, llmIDs)
	for i := range llms {
		llm := &llms[i]
		item := CatalogItem{Type: CatalogItemLLM, ID: strconv.FormatUint(uint64(llm.ID), 10)}
		item.Attributes = CatalogItemAttributes{
			Name:             llm.Name,
			ShortDescription: llm.ShortDescription,
			LongDescription:  llm.LongDescription,
			LogoURL:          llm.LogoURL,
			Kind:             string(llm.Vendor),
			PrivacyScore:     intPtr(llm.PrivacyScore),
			Tags:             []string{},
			Catalogs:         catalogRefs(llmMembership[llm.ID], llmCatNames),
			CreatedAt:        timePtr(llm.CreatedAt),
			UpdatedAt:        timePtr(llm.UpdatedAt),
			DefaultModel:     llm.DefaultModel,
			AllowedModels:    llm.AllowedModels,
		}
		if llmGoverned != nil {
			item.GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeLLM, llmGoverned[item.ID])
		}
		out.Items = append(out.Items, item)
	}

	// --- Data sources ---
	dataCatalogues, err := user.GetAccessibleDataCatalogues(db)
	if err != nil {
		return nil, err
	}
	dataCatIDs := make([]uint, 0, len(dataCatalogues))
	dataCatNames := make(map[uint]string, len(dataCatalogues))
	for _, cat := range dataCatalogues {
		dataCatIDs = append(dataCatIDs, cat.ID)
		dataCatNames[cat.ID] = cat.Name
		out.Catalogs = append(out.Catalogs, CatalogFilterOption{Type: CatalogItemDatasource, ID: strconv.FormatUint(uint64(cat.ID), 10), Name: cat.Name})
	}
	datasources, err := user.GetAccessibleDataSources(db)
	if err != nil {
		return nil, err
	}
	dsMembership, err := models.DatasourceCatalogueMemberships(db, dataCatIDs)
	if err != nil {
		return nil, err
	}
	dsIDs := make([]string, 0, len(datasources))
	for _, ds := range datasources {
		dsIDs = append(dsIDs, strconv.FormatUint(uint64(ds.ID), 10))
	}
	dsGoverned := a.governedMetadataFor(models.GovernedObjectTypeDatasource, dsIDs)
	// Tags are not loaded by the accessible query; one query for all of them.
	dsTags, err := datasourceTagNames(db, datasources)
	if err != nil {
		return nil, err
	}
	for i := range datasources {
		ds := &datasources[i]
		item := CatalogItem{Type: CatalogItemDatasource, ID: strconv.FormatUint(uint64(ds.ID), 10)}
		tags := dsTags[ds.ID]
		if tags == nil {
			tags = []string{}
		}
		item.Attributes = CatalogItemAttributes{
			Name:               ds.Name,
			ShortDescription:   ds.ShortDescription,
			LongDescription:    ds.LongDescription,
			LogoURL:            ds.Icon,
			Kind:               ds.DBSourceType,
			PrivacyScore:       intPtr(ds.PrivacyScore),
			CommunitySubmitted: ds.CommunitySubmitted,
			Tags:               tags,
			Catalogs:           catalogRefs(dsMembership[ds.ID], dataCatNames),
			CreatedAt:          timePtr(ds.CreatedAt),
			UpdatedAt:          timePtr(ds.UpdatedAt),
			EmbedVendor:        string(ds.EmbedVendor),
			EmbedModel:         ds.EmbedModel,
		}
		if dsGoverned != nil {
			item.GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeDatasource, dsGoverned[item.ID])
		}
		out.Items = append(out.Items, item)
	}

	// --- Tools ---
	toolCatalogues, err := user.GetAccessibleToolCatalogues(db)
	if err != nil {
		return nil, err
	}
	toolCatIDs := make([]uint, 0, len(toolCatalogues))
	toolCatNames := make(map[uint]string, len(toolCatalogues))
	for _, cat := range toolCatalogues {
		toolCatIDs = append(toolCatIDs, cat.ID)
		toolCatNames[cat.ID] = cat.Name
		out.Catalogs = append(out.Catalogs, CatalogFilterOption{Type: CatalogItemTool, ID: strconv.FormatUint(uint64(cat.ID), 10), Name: cat.Name})
	}
	tools, err := user.GetAccessibleTools(db)
	if err != nil {
		return nil, err
	}
	toolMembership, err := models.ToolCatalogueMemberships(db, toolCatIDs)
	if err != nil {
		return nil, err
	}
	toolIDs := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolIDs = append(toolIDs, strconv.FormatUint(uint64(tool.ID), 10))
	}
	toolGoverned := a.governedMetadataFor(models.GovernedObjectTypeTool, toolIDs)
	for i := range tools {
		tool := &tools[i]
		// The per-catalog page hides tools that are switched off; the
		// accessible query does not filter on it, so it is applied here.
		if !tool.Active {
			continue
		}
		item := CatalogItem{Type: CatalogItemTool, ID: strconv.FormatUint(uint64(tool.ID), 10)}
		item.Attributes = CatalogItemAttributes{
			Name:               tool.Name,
			ShortDescription:   tool.Description,
			Kind:               tool.ToolType,
			PrivacyScore:       intPtr(tool.PrivacyScore),
			CommunitySubmitted: tool.CommunitySubmitted,
			Tags:               []string{},
			Catalogs:           catalogRefs(toolMembership[tool.ID], toolCatNames),
			CreatedAt:          timePtr(tool.CreatedAt),
			UpdatedAt:          timePtr(tool.UpdatedAt),
			Operations:         tool.GetOperations(),
		}
		if toolGoverned != nil {
			item.GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeTool, toolGoverned[item.ID])
		}
		out.Items = append(out.Items, item)
	}

	// --- Plugin resources ---
	resourceTypes, err := a.accessiblePluginResourceInstances(c, user)
	if err != nil {
		return nil, err
	}
	for _, rt := range resourceTypes {
		ref := CatalogResourceType{PluginID: rt.Type.PluginID, Slug: rt.Type.Slug, Name: sanitizeString(rt.Type.Name), Icon: sanitizeString(rt.Type.Icon)}
		out.ResourceTypes = append(out.ResourceTypes, ref)
		kind := strconv.FormatUint(uint64(rt.Type.PluginID), 10) + ":" + rt.Type.Slug
		objectType := models.PluginResourceObjectType(rt.Type.PluginID, rt.Type.Slug)
		for _, inst := range rt.Instances {
			refCopy := ref
			item := CatalogItem{Type: CatalogItemPluginResource, ID: inst.Id}
			item.Attributes = CatalogItemAttributes{
				Name:             sanitizeString(inst.Name),
				ShortDescription: sanitizeString(inst.Description),
				Kind:             kind,
				KindLabel:        refCopy.Name,
				Tags:             []string{},
				Catalogs:         []CatalogRef{},
				ResourceType:     &refCopy,
			}
			if rt.Type.HasPrivacyScore {
				item.Attributes.PrivacyScore = intPtr(int(inst.PrivacyScore))
			}
			if rt.Governed != nil {
				item.GovernedMetadata = a.portalGovernedView(objectType, rt.Governed[inst.Id])
			}
			out.Items = append(out.Items, item)
		}
	}

	return out, nil
}

// datasourceTagNames loads tag names for the given data sources in one query.
func datasourceTagNames(db *gorm.DB, datasources []models.Datasource) (map[uint][]string, error) {
	out := make(map[uint][]string)
	if len(datasources) == 0 {
		return out, nil
	}
	ids := make([]uint, 0, len(datasources))
	for _, ds := range datasources {
		ids = append(ids, ds.ID)
	}
	var rows []struct {
		DatasourceID uint
		Name         string
	}
	err := db.Table("datasource_tags").
		Select("datasource_tags.datasource_id AS datasource_id, tags.name AS name").
		Joins("JOIN tags ON tags.id = datasource_tags.tag_id").
		Where("datasource_tags.datasource_id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.DatasourceID] = append(out[row.DatasourceID], row.Name)
	}
	return out, nil
}

// accessibleResourceType is one plugin resource type with the instances the
// caller may use.
type accessibleResourceType struct {
	Type      models.PluginResourceType
	Instances []*pb.ResourceInstanceProto
	Governed  map[string]*models.ObjectMetadata
}

// accessiblePluginResourceInstances returns every active plugin resource type
// with the active instances the caller can use: all of them for callers who
// manage teams, otherwise the instances granted to the caller's teams. It is
// the visibility rule of GET /common/accessible-plugin-resources, shared so
// the catalog and the AppBuilder never disagree. Types whose plugin cannot be
// reached are returned with no instances.
func (a *API) accessiblePluginResourceInstances(c *gin.Context, user *models.User) ([]accessibleResourceType, error) {
	types, err := a.service.GetPluginResourceTypes()
	if err != nil || len(types) == 0 || a.service.AIStudioPluginManager == nil {
		return nil, nil
	}

	var accessibleByType map[uint]map[string]bool
	if !authz.Can(c, authz.Write("groups")) {
		allAccessible, err := a.service.GetAllAccessiblePluginResources(user.ID)
		if err != nil {
			// Fail closed: an unknown grant set means no plugin resources.
			return nil, nil
		}
		accessibleByType = make(map[uint]map[string]bool)
		for _, gpr := range allAccessible {
			if accessibleByType[gpr.PluginResourceTypeID] == nil {
				accessibleByType[gpr.PluginResourceTypeID] = make(map[string]bool)
			}
			accessibleByType[gpr.PluginResourceTypeID][gpr.InstanceID] = true
		}
	}

	// Each type is one RPC to its plugin; they run concurrently and each
	// goroutine writes only its own slot.
	result := make([]accessibleResourceType, len(types))
	var wg sync.WaitGroup
	for i, rt := range types {
		result[i].Type = rt
		wg.Add(1)
		go func(idx int, rt models.PluginResourceType) {
			defer wg.Done()
			protoInstances, err := a.service.AIStudioPluginManager.ListResourceInstances(rt.PluginID, rt.Slug)
			if err != nil {
				return
			}
			accessibleSet := accessibleByType[rt.ID] // nil for team managers
			instances := make([]*pb.ResourceInstanceProto, 0, len(protoInstances))
			for _, inst := range protoInstances {
				if !inst.IsActive {
					continue
				}
				if accessibleByType != nil && !accessibleSet[inst.Id] {
					continue
				}
				instances = append(instances, inst)
			}
			result[idx].Instances = instances
			if rt.SupportsMetadata && len(instances) > 0 {
				ids := make([]string, 0, len(instances))
				for _, inst := range instances {
					ids = append(ids, inst.Id)
				}
				result[idx].Governed = a.governedMetadataFor(models.PluginResourceObjectType(rt.PluginID, rt.Slug), ids)
			}
		}(i, rt)
	}
	wg.Wait()
	return result, nil
}

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
// @Description Every LLM provider, data source, tool and plugin resource the authenticated user can build an app with, in one item shape, with the catalogs each is available through. Visibility follows the user's teams and their catalogs.
// @Tags common
// @Produce json
// @Success 200 {object} CatalogListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/catalog [get]
func (a *API) getPortalCatalog(c *gin.Context) {
	user, ok := portalUser(c)
	if !ok {
		return
	}
	catalog, err := a.buildPortalCatalog(c, user)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	counts := map[string]int{CatalogItemLLM: 0, CatalogItemDatasource: 0, CatalogItemTool: 0, CatalogItemPluginResource: 0}
	for _, item := range catalog.Items {
		counts[item.Type]++
	}
	c.JSON(http.StatusOK, CatalogListResponse{
		Data: catalog.Items,
		Meta: CatalogListMeta{
			Total:         len(catalog.Items),
			Counts:        counts,
			Catalogs:      catalog.Catalogs,
			ResourceTypes: catalog.ResourceTypes,
		},
	})
}

// findCatalogItem answers a detail request: the item if the caller can see
// it, otherwise 404. Anything outside the caller's visibility is "not found"
// rather than "forbidden" so the endpoint never confirms that an id exists.
func (a *API) findCatalogItem(c *gin.Context, itemType, id string) (*CatalogItem, bool) {
	user, ok := portalUser(c)
	if !ok {
		return nil, false
	}
	catalog, err := a.buildPortalCatalog(c, user)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return nil, false
	}
	for i := range catalog.Items {
		item := &catalog.Items[i]
		if item.Type == itemType && item.ID == id {
			return item, true
		}
	}
	simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
	return nil, false
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
	kind := c.Param("plugin_id") + ":" + c.Param("slug")
	item, ok := a.findCatalogItem(c, CatalogItemPluginResource, c.Param("id"))
	if !ok {
		return
	}
	if item.Attributes.Kind != kind {
		simpleError(c, http.StatusNotFound, "Not Found", "No such item is available to you")
		return
	}
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
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
	for _, app := range apps {
		appIDs = append(appIDs, app.ID)
	}
	activity, err := analytics.GetAppActivity(a.service.DB, appIDs, now.AddDate(0, 0, -30))
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	data := make(map[string]AppUsageSummary, len(apps))
	for _, app := range apps {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		if app.BudgetStartDate != nil {
			start = *app.BudgetStartDate
		}
		spent := 0.0
		if a.service.Budget != nil {
			if value, err := a.service.Budget.GetMonthlySpending(app.ID, start, now); err == nil {
				spent = value
			}
		}
		summary := AppUsageSummary{
			AppID:           strconv.FormatUint(uint64(app.ID), 10),
			CurrentSpend:    spent,
			MonthlyBudget:   app.MonthlyBudget,
			BudgetStartDate: start,
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
	c.JSON(http.StatusOK, AppUsageSummaryResponse{Data: data, SpendTracked: budget.IsEnterpriseAvailable()})
}
