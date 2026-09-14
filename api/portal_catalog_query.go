package api

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Search, filtering, sorting and paging for GET /common/catalog.
//
// The database-backed types (LLM providers, data sources, tools) are
// filtered in SQL on top of the composable accessible-object queries
// (models.Accessible*Query), and counted for the facets with aggregates, so
// the rows loaded are the ones that match. When the request is fixed to one
// of those types the ordering and the page are SQL too; the mixed "all"
// view merges the matching rows of every type in memory (plugin resources
// come from their plugins over RPC and are filtered here in any case).
// Query string:
//
//	q          case-insensitive terms; every term must appear somewhere in
//	           the item's name, descriptions, kind (code or vendor label),
//	           type, model names, operations, tags or catalog names
//	type       llm | datasource | tool | plugin_resource
//	kind       vendor code / store type / protocol / "<plugin id>:<slug>"
//	privacy    public | internal | confidential | restricted (the bands of
//	           ui/admin-frontend/src/admin/components/common/privacy/privacyLevels.js)
//	catalog    "<type>:<catalog id>"
//	community  true to keep community submissions only
//	sort       newest (default) | name | privacy_asc | privacy_desc
//	page       1-based, default 1
//	page_size  default 25, at most 100

const (
	catalogDefaultPageSize = 25
	catalogMaxPageSize     = 100
)

// Privacy bands, the same as the frontend's privacyLevels.js.
var catalogPrivacyBands = map[string][2]int{
	"public":       {0, 25},
	"internal":     {26, 50},
	"confidential": {51, 75},
	"restricted":   {76, 100},
}

var catalogSorts = map[string]bool{"newest": true, "name": true, "privacy_asc": true, "privacy_desc": true}

// Display names for the built-in LLM vendor codes, so "aws" finds Bedrock
// providers the way the UI labels them.
var catalogVendorNames = map[string]string{
	"openai": "OpenAI", "anthropic": "Anthropic", "vertex": "Vertex AI", "google_ai": "Google AI",
	"huggingface": "HuggingFace", "ollama": "Ollama", "bedrock": "AWS Bedrock",
}

var catalogTypeNames = map[string]string{
	CatalogItemLLM:            "LLM provider",
	CatalogItemDatasource:     "data source",
	CatalogItemTool:           "tool",
	CatalogItemPluginResource: "resource",
}

type catalogQuery struct {
	Terms     []string
	Type      string
	Kind      string
	Privacy   string
	Catalog   string // "<type>:<id>"
	Community bool
	Sort      string
	Page      int
	PageSize  int
}

// parseCatalogQuery reads the query string; it returns false having written
// a 400 for a value outside the vocabulary above.
func parseCatalogQuery(c *gin.Context) (catalogQuery, bool) {
	q := catalogQuery{
		Type:      strings.TrimSpace(c.Query("type")),
		Kind:      strings.TrimSpace(c.Query("kind")),
		Privacy:   strings.ToLower(strings.TrimSpace(c.Query("privacy"))),
		Catalog:   strings.TrimSpace(c.Query("catalog")),
		Community: c.Query("community") == "true" || c.Query("community") == "1",
		Sort:      strings.TrimSpace(c.Query("sort")),
		Page:      1,
		PageSize:  catalogDefaultPageSize,
	}
	if raw := strings.ToLower(strings.TrimSpace(c.Query("q"))); raw != "" {
		q.Terms = strings.Fields(raw)
	}
	if q.Sort == "" {
		q.Sort = "newest"
	}
	if !catalogSorts[q.Sort] {
		simpleError(c, http.StatusBadRequest, "Bad Request", "unsupported sort: "+q.Sort)
		return q, false
	}
	if q.Privacy != "" {
		if _, ok := catalogPrivacyBands[q.Privacy]; !ok {
			simpleError(c, http.StatusBadRequest, "Bad Request", "unsupported privacy level: "+q.Privacy)
			return q, false
		}
	}
	if q.Type != "" {
		if _, ok := catalogTypeNames[q.Type]; !ok {
			simpleError(c, http.StatusBadRequest, "Bad Request", "unsupported type: "+q.Type)
			return q, false
		}
	}
	if raw := c.Query("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid page number")
			return q, false
		}
		q.Page = n
	}
	if raw := c.Query("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid page size")
			return q, false
		}
		if n > catalogMaxPageSize {
			n = catalogMaxPageSize
		}
		q.PageSize = n
	}
	return q, true
}

// catalogType and catalogID of the catalog filter ("<type>:<id>").
func (q catalogQuery) catalogFilter() (string, string) {
	if q.Catalog == "" {
		return "", ""
	}
	t, id, _ := strings.Cut(q.Catalog, ":")
	return t, id
}

// --- SQL side: database-backed types --------------------------------------

// catalogSource describes one database-backed asset type to the query layer:
// which columns carry its kind, privacy level and community flag, which
// columns a search term is matched against, and how its catalogues are
// named, on top of the accessible-object base query.
type catalogSource struct {
	typ              string
	table            string
	kindCol          string
	privacyCol       string
	communityCol     string // empty when the type has no community submissions
	catalogueIDCol   string
	catalogueNameCol string
	searchCols       []string
	// extraSearch is an optional clause with one "?" for the LIKE pattern
	// (tags live in a join table).
	extraSearch string
	base        func(db *gorm.DB, userID uint) *gorm.DB
}

var catalogSources = []catalogSource{
	{
		typ: CatalogItemLLM, table: "llms",
		kindCol: "llms.vendor", privacyCol: "llms.privacy_score",
		catalogueIDCol: "catalogue_llms.catalogue_id", catalogueNameCol: "catalogues.name",
		searchCols: []string{"llms.name", "llms.short_description", "llms.long_description", "llms.default_model", "llms.allowed_models"},
		base:       models.AccessibleLLMQuery,
	},
	{
		typ: CatalogItemDatasource, table: "datasources",
		kindCol: "datasources.db_source_type", privacyCol: "datasources.privacy_score", communityCol: "datasources.community_submitted",
		catalogueIDCol: "data_catalogue_data_sources.data_catalogue_id", catalogueNameCol: "data_catalogues.name",
		searchCols:  []string{"datasources.name", "datasources.short_description", "datasources.long_description", "datasources.embed_model"},
		extraSearch: "EXISTS (SELECT 1 FROM datasource_tags dt JOIN tags ON tags.id = dt.tag_id WHERE dt.datasource_id = datasources.id AND LOWER(tags.name) LIKE ? ESCAPE '\\')",
		base:        models.AccessibleDatasourceQuery,
	},
	{
		typ: CatalogItemTool, table: "tools",
		kindCol: "tools.tool_type", privacyCol: "tools.privacy_score", communityCol: "tools.community_submitted",
		catalogueIDCol: "tool_catalogue_tools.tool_catalogue_id", catalogueNameCol: "tool_catalogues.name",
		searchCols: []string{"tools.name", "tools.description", "tools.available_operations"},
		base:       models.AccessibleToolQuery,
	},
}

func catalogSourceFor(typ string) *catalogSource {
	for i := range catalogSources {
		if catalogSources[i].typ == typ {
			return &catalogSources[i]
		}
	}
	return nil
}

// applies reports whether any row of this type can match the query: a type,
// community or catalog filter for another type rules the whole type out
// without a query.
func (src catalogSource) applies(q catalogQuery) bool {
	if q.Type != "" && q.Type != src.typ {
		return false
	}
	if q.Community && src.communityCol == "" {
		return false
	}
	if t, _ := q.catalogFilter(); t != "" && t != src.typ {
		return false
	}
	return true
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// vendorCodesMatching returns the LLM vendor codes whose display name
// contains the term ("aws" -> bedrock).
func vendorCodesMatching(term string) []string {
	codes := []string{}
	for code, label := range catalogVendorNames {
		if strings.Contains(strings.ToLower(label), term) {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return codes
}

// scope applies the query's filters to the type's base query.
func (src catalogSource) scope(q catalogQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.Kind != "" {
			db = db.Where(src.kindCol+" = ?", q.Kind)
		}
		if q.Privacy != "" {
			band := catalogPrivacyBands[q.Privacy]
			db = db.Where(src.privacyCol+" BETWEEN ? AND ?", band[0], band[1])
		}
		if q.Community {
			db = db.Where(src.communityCol+" = ?", true)
		}
		if _, id := q.catalogFilter(); id != "" {
			db = db.Where(src.catalogueIDCol+" = ?", id)
		}
		typeName := strings.ToLower(catalogTypeNames[src.typ])
		for _, term := range q.Terms {
			if strings.Contains(typeName, term) {
				continue // "tool" is satisfied by every tool
			}
			pattern := "%" + likeEscaper.Replace(term) + "%"
			clauses := make([]string, 0, len(src.searchCols)+3)
			args := make([]interface{}, 0, len(src.searchCols)+3)
			for _, col := range src.searchCols {
				clauses = append(clauses, "LOWER("+col+") LIKE ? ESCAPE '\\'")
				args = append(args, pattern)
			}
			clauses = append(clauses, "LOWER("+src.catalogueNameCol+") LIKE ? ESCAPE '\\'")
			args = append(args, pattern)
			if src.extraSearch != "" {
				clauses = append(clauses, src.extraSearch)
				args = append(args, pattern)
			}
			if src.typ == CatalogItemLLM {
				if codes := vendorCodesMatching(term); len(codes) > 0 {
					clauses = append(clauses, src.kindCol+" IN ?")
					args = append(args, codes)
				}
			}
			db = db.Where("("+strings.Join(clauses, " OR ")+")", args...)
		}
		return db
	}
}

// order is the SQL ORDER BY for a sort key, used when the request is fixed
// to this type and the page can be taken in SQL.
func (src catalogSource) order(sortKey string) string {
	name := "LOWER(" + src.table + ".name) ASC"
	switch sortKey {
	case "name":
		return name
	case "privacy_asc":
		return src.privacyCol + " ASC, " + name
	case "privacy_desc":
		return src.privacyCol + " DESC, " + name
	default:
		return src.table + ".created_at DESC, " + name
	}
}

// --- in-memory side: merging types, plugin resources ------------------------

// catalogHaystack is everything a search term can match, lower-cased. It is
// the in-memory twin of catalogSource.scope, used for plugin resources.
func catalogHaystack(item *CatalogItem) string {
	a := &item.Attributes
	parts := []string{a.Name, a.ShortDescription, a.LongDescription, a.Kind, a.KindLabel,
		catalogVendorNames[a.Kind], catalogTypeNames[item.Type], a.DefaultModel, a.EmbedVendor, a.EmbedModel}
	parts = append(parts, a.AllowedModels...)
	parts = append(parts, a.Operations...)
	parts = append(parts, a.Tags...)
	for _, ref := range a.Catalogs {
		parts = append(parts, ref.Name)
	}
	if a.ResourceType != nil {
		parts = append(parts, a.ResourceType.Name)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func (q catalogQuery) matches(item *CatalogItem) bool {
	a := &item.Attributes
	if q.Type != "" && item.Type != q.Type {
		return false
	}
	if q.Kind != "" && a.Kind != q.Kind {
		return false
	}
	if q.Privacy != "" {
		band := catalogPrivacyBands[q.Privacy]
		if a.PrivacyScore == nil || *a.PrivacyScore < band[0] || *a.PrivacyScore > band[1] {
			return false
		}
	}
	if catalogType, catalogID := q.catalogFilter(); catalogType != "" {
		if item.Type != catalogType {
			return false
		}
		found := false
		for _, ref := range a.Catalogs {
			if ref.ID == catalogID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if q.Community && !a.CommunitySubmitted {
		return false
	}
	if len(q.Terms) > 0 {
		hay := catalogHaystack(item)
		for _, term := range q.Terms {
			if !strings.Contains(hay, term) {
				return false
			}
		}
	}
	return true
}

// sortCatalogItems orders in place. Newest first is the default; items with
// no timestamp (plugin resources) come last, ties break on name.
func sortCatalogItems(items []CatalogItem, by string) {
	name := func(i int) string { return strings.ToLower(items[i].Attributes.Name) }
	sort.SliceStable(items, func(i, j int) bool {
		ai, aj := &items[i].Attributes, &items[j].Attributes
		switch by {
		case "name":
			return name(i) < name(j)
		case "privacy_asc", "privacy_desc":
			pi, pj := ai.PrivacyScore, aj.PrivacyScore
			if (pi == nil) != (pj == nil) {
				return pi != nil // unset last either way
			}
			if pi != nil && *pi != *pj {
				if by == "privacy_asc" {
					return *pi < *pj
				}
				return *pi > *pj
			}
			return name(i) < name(j)
		default: // newest
			ci, cj := ai.CreatedAt, aj.CreatedAt
			if (ci == nil) != (cj == nil) {
				return ci != nil
			}
			if ci != nil && !ci.Equal(*cj) {
				return ci.After(*cj)
			}
			return name(i) < name(j)
		}
	})
}

// pageOf returns the requested page and the page count (at least 1).
func pageOf(items []CatalogItem, page, size int) ([]CatalogItem, int) {
	totalPages := totalPagesFor(len(items), size)
	start := (page - 1) * size
	if start >= len(items) {
		return []CatalogItem{}, totalPages
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], totalPages
}

func totalPagesFor(total, size int) int {
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	return pages
}

// CatalogKindFacet is one kind (vendor, store type, protocol, resource type)
// present in the caller's accessible set, for the Kind filter.
type CatalogKindFacet struct {
	Type  string `json:"type"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

func sortKindFacets(kinds []CatalogKindFacet) {
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].Type != kinds[j].Type {
			return kinds[i].Type < kinds[j].Type
		}
		li, lj := kinds[i].Label, kinds[j].Label
		if li == "" {
			li = kinds[i].Kind
		}
		if lj == "" {
			lj = kinds[j].Kind
		}
		return strings.ToLower(li) < strings.ToLower(lj)
	})
}

// --- output hygiene ---------------------------------------------------------

var markupPattern = regexp.MustCompile(`<[^>]*>`)

// cleanText removes markup from an administrator- or submitter-authored
// string before it leaves the API. The portal renders these as text (React
// escapes them), so this is defence in depth for any other consumer of the
// endpoint; unlike entity-encoding it leaves apostrophes and ampersands as
// typed, so "Martin's GPT" and "R&D" read as written.
func cleanText(s string) string {
	if !strings.ContainsAny(s, "<>") {
		return s
	}
	s = markupPattern.ReplaceAllString(s, "")
	return strings.NewReplacer("<", "", ">", "").Replace(s)
}

func cleanTexts(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = cleanText(v)
	}
	return out
}
