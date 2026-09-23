package services

import (
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"gorm.io/gorm"
)

// ListOptions carries the optional search and sort of a plain list endpoint
// (GET /llms, /tools, ...). Search is a case-insensitive substring match over
// the type's name and description-like columns; Sort is a column name from
// the type's whitelist, with Desc set for "-field". The API layer validates
// the sort field (api/list_query.go) so the column reaching here is trusted.
//
// The zero value means "no search, default order", which is what every list
// did before these options existed.
type ListOptions struct {
	Search   string
	Sort     string
	SortDesc bool
}

// Scopes turns the options into GORM scopes for the model's GetAll.
func (o ListOptions) Scopes(searchColumns ...string) []func(*gorm.DB) *gorm.DB {
	var scopes []func(*gorm.DB) *gorm.DB
	if term := strings.TrimSpace(o.Search); term != "" && len(searchColumns) > 0 {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return applySearch(db, searchColumns, term) })
	}
	if o.Sort != "" {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return applySort(db, o.Sort, o.SortDesc) })
	}
	return scopes
}

// firstListOptions is for the variadic service signatures: callers that
// predate search/sort pass nothing and get the zero value.
func firstListOptions(opts []ListOptions) ListOptions {
	if len(opts) == 0 {
		return ListOptions{}
	}
	return opts[0]
}

// likePattern escapes the LIKE wildcards in a user-supplied term so "50%"
// matches a literal percent sign, and wraps it for a substring match. The
// escape character is a backslash, declared with ESCAPE in the clause.
func likePattern(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + strings.ToLower(r.Replace(term)) + "%"
}

// applySearch adds a case-insensitive substring predicate over cols.
// LOWER(col) LIKE ? is portable: SQLite's LIKE is already case-insensitive
// for ASCII, Postgres's is not, and both accept ESCAPE.
func applySearch(db *gorm.DB, cols []string, term string) *gorm.DB {
	term = strings.TrimSpace(term)
	if term == "" || len(cols) == 0 {
		return db
	}
	pattern := likePattern(term)
	clauses := make([]string, 0, len(cols))
	args := make([]interface{}, 0, len(cols))
	for _, col := range cols {
		clauses = append(clauses, "LOWER("+col+") LIKE ? ESCAPE '\\'")
		args = append(args, pattern)
	}
	return db.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

// applySort orders by one column, with id as the tiebreaker so pages stay
// stable when many rows share a value (every LLM created in one import has
// the same created_at second).
func applySort(db *gorm.DB, column string, desc bool) *gorm.DB {
	if column == "" {
		return db
	}
	dir := " ASC"
	if desc {
		dir = " DESC"
	}
	db = db.Order(column + dir)
	if column != "id" {
		db = db.Order("id ASC")
	}
	return db
}

// ListModelRouters is the searchable, sortable form of
// ModelRouterService.ListRouters. A plain list (no search, no sort) is
// exactly ListRouters and goes through the interface, so an edition stub or
// a test double keeps its say. The router service interface is shared with
// the Enterprise submodule, so rather than widen it for search and sort the
// filtered query runs here against the same model, behind the same edition
// gate: Community Edition answers ErrEnterpriseFeature as ListRouters does.
func (s *Service) ListModelRouters(pageSize int, pageNumber int, all bool, opts ...ListOptions) ([]models.ModelRouter, int64, int, error) {
	o := firstListOptions(opts)
	if o.Search == "" && o.Sort == "" && s.ModelRouterService != nil {
		return s.ModelRouterService.ListRouters(pageSize, pageNumber, all)
	}
	if !model_router.IsEnterpriseAvailable() {
		return nil, 0, 0, model_router.ErrEnterpriseFeature
	}
	var routers models.ModelRouters
	totalCount, totalPages, err := routers.GetAll(s.DB, pageSize, pageNumber, all,
		firstListOptions(opts).Scopes("name", "description")...)
	if err != nil {
		return nil, 0, 0, err
	}
	return routers, totalCount, totalPages, nil
}

// ListSemanticRouters is the searchable, sortable router list.
func (s *Service) ListSemanticRouters(pageSize int, pageNumber int, all bool, opts ...ListOptions) ([]models.SemanticRouter, int64, int, error) {
	svc := s.SemanticRouterService
	if svc == nil {
		svc = semantic_router.NewService(s.DB)
	}
	return svc.ListRouters(pageSize, pageNumber, all, firstListOptions(opts).Scopes("name", "description")...)
}
