package models

import (
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// Model Routers as portal assets: how private a router is, and which model
// names a client can ask it for.

// ModelRouterPrivacySQL is a router's privacy score in SQL, as a correlated
// subquery on model_routers.id: the lowest score among the active LLMs behind
// its active vendors. A router is only as private as the least private LLM it
// may send a request to.
const ModelRouterPrivacySQL = "(SELECT MIN(rl.privacy_score) FROM pool_vendors rpv " +
	"JOIN model_pools rmp ON rmp.id = rpv.pool_id AND rmp.deleted_at IS NULL " +
	"JOIN llms rl ON rl.id = rpv.llm_id AND rl.active = TRUE AND rl.deleted_at IS NULL " +
	"WHERE rmp.router_id = model_routers.id AND rpv.active = TRUE AND rpv.deleted_at IS NULL)"

// ModelRouterPrivacyScores maps router ids to their privacy score (see
// ModelRouterPrivacySQL). Routers that reach no active LLM are absent.
func ModelRouterPrivacyScores(db *gorm.DB, ids []uint) (map[uint]int, error) {
	out := map[uint]int{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID    uint
		Score *int
	}
	err := db.Model(&ModelRouter{}).
		Select("model_routers.id AS id, "+ModelRouterPrivacySQL+" AS score").
		Where("model_routers.id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Score != nil {
			out[r.ID] = *r.Score
		}
	}
	return out, nil
}

// AdvertisedModels lists the model names a client can ask this router for by
// name: the literal (glob-free) entries of its pool patterns and the source
// models of its vendor mappings. Pools must be loaded. The gateway serves a
// router's models as "{slug}/{model}".
func (r *ModelRouter) AdvertisedModels() []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] || strings.ContainsAny(m, "*?[") {
			return
		}
		seen[m] = true
		out = append(out, m)
	}
	for _, p := range r.Pools {
		if p == nil {
			continue
		}
		for _, pat := range strings.Split(p.ModelPattern, ",") {
			add(pat)
		}
		for _, v := range p.Vendors {
			if v == nil {
				continue
			}
			for _, m := range v.Mappings {
				if m != nil {
					add(m.SourceModel)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// ErrUnsafeLogoURL is returned for a logo URL a browser could be tricked by
// (javascript:, data:, protocol-relative and the like).
var ErrUnsafeLogoURL = errors.New("logo_url must be an http(s) URL or a same-origin path")

// SafeLogoURL returns u when it is safe to put in an <img src> the portal
// renders: an http(s) URL or a same-origin path (a single leading "/", no
// scheme). Anything else, such as a javascript: URL, becomes "".
func SafeLogoURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.ContainsAny(u, " \t\r\n") {
		return ""
	}
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") && !strings.Contains(u, "://") {
		return u
	}
	return ""
}

// CheckLogoURL refuses a non-empty logo URL SafeLogoURL would drop.
func CheckLogoURL(u string) error {
	if strings.TrimSpace(u) != "" && SafeLogoURL(u) == "" {
		return ErrUnsafeLogoURL
	}
	return nil
}
