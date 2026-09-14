package api

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// Search and sort on the plain list endpoints (GET /llms, /tools, ...).
//
//	?search=<term>   case-insensitive substring over the type's name and
//	                 description-like columns (see the service wrappers)
//	?sort=<field>    ascending; ?sort=-<field> descending
//
// The sort field must be in the endpoint's whitelist; anything else is a 400
// so a typo never reaches the database as an ORDER BY. The default order,
// id ascending, is what every list produced before these parameters existed.

// sortFields is an endpoint's whitelist: API field name -> column. Most
// entries are identity; an alias (model prices' "name" -> model_name) is the
// exception so every list accepts the same vocabulary.
type sortFields map[string]string

// sortable builds an identity whitelist from column names.
func sortable(cols ...string) sortFields {
	f := make(sortFields, len(cols))
	for _, col := range cols {
		f[col] = col
	}
	return f
}

// alias adds an API name for an existing column.
func (f sortFields) alias(name, col string) sortFields {
	f[name] = col
	return f
}

// The whitelists, one per list endpoint. Only columns that exist on the
// table are listed: sorting by "active" on filters, which have no live
// switch, is a 400 like any other unknown field.
var (
	llmSortFields         = sortable("id", "name", "created_at", "updated_at", "privacy_score", "active", "vendor")
	toolSortFields        = sortable("id", "name", "created_at", "updated_at", "privacy_score", "active")
	datasourceSortFields  = sortable("id", "name", "created_at", "updated_at", "privacy_score", "active")
	filterSortFields      = sortable("id", "name", "created_at", "updated_at")
	secretSortFields      = sortable("id", "var_name", "created_at", "updated_at").alias("name", "var_name")
	modelPriceSortFields  = sortable("id", "model_name", "vendor", "created_at", "updated_at").alias("name", "model_name")
	catalogueSortFields   = sortable("id", "name", "created_at", "updated_at")
	modelRouterSortFields = sortable("id", "name", "created_at", "updated_at", "active")
)

// parseListQuery reads ?search and ?sort. It returns false having written a
// 400 when the sort field is not in allowed.
func parseListQuery(c *gin.Context, allowed sortFields) (services.ListOptions, bool) {
	opts := services.ListOptions{Search: strings.TrimSpace(c.Query("search"))}

	sort := strings.TrimSpace(c.Query("sort"))
	if sort == "" {
		return opts, true
	}
	desc := false
	if strings.HasPrefix(sort, "-") {
		desc = true
		sort = sort[1:]
	}
	col, ok := allowed[sort]
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "unsupported sort field: " + sort}},
		})
		return opts, false
	}
	opts.Sort = col
	opts.SortDesc = desc
	return opts, true
}
