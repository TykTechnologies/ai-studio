package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Search, filtering, sorting and paging for GET /common/catalog.
//
// The client never receives more than one page. The caller's accessible set
// is assembled once per request (buildPortalCatalog), the facets the filter
// controls need (counts per type, kinds, catalogs) are taken from that full
// set, and the query below narrows, orders and pages it. Query string:
//
//	q          case-insensitive terms; every term must appear somewhere in
//	           the item's name, descriptions, kind, type, model names,
//	           operations, tags or catalog names
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

// haystack is everything a search term can match, lower-cased.
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
	if q.Catalog != "" {
		catalogType, catalogID, _ := strings.Cut(q.Catalog, ":")
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
	totalPages := (len(items) + size - 1) / size
	if totalPages < 1 {
		totalPages = 1
	}
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

// CatalogKindFacet is one kind (vendor, store type, protocol, resource type)
// present in the caller's accessible set, for the Kind filter.
type CatalogKindFacet struct {
	Type  string `json:"type"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

// catalogFacets summarises the full accessible set: how many items of each
// type, and which kinds occur (with counts), ordered by type then label.
func catalogFacets(items []CatalogItem) (map[string]int, []CatalogKindFacet) {
	counts := map[string]int{CatalogItemLLM: 0, CatalogItemDatasource: 0, CatalogItemTool: 0, CatalogItemPluginResource: 0}
	index := map[string]int{}
	kinds := []CatalogKindFacet{}
	for i := range items {
		item := &items[i]
		counts[item.Type]++
		kind := item.Attributes.Kind
		if kind == "" {
			continue
		}
		key := item.Type + "\x00" + kind
		if at, ok := index[key]; ok {
			kinds[at].Count++
			continue
		}
		label := item.Attributes.KindLabel
		if label == "" {
			label = catalogVendorNames[kind]
		}
		index[key] = len(kinds)
		kinds = append(kinds, CatalogKindFacet{Type: item.Type, Kind: kind, Label: label, Count: 1})
	}
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
	return counts, kinds
}
